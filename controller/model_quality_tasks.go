package controller

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/qualityinspect"
	"gorm.io/gorm"
)

type modelQualityScheduleHandler struct{}

func (modelQualityScheduleHandler) Type() string            { return model.ModelQualityScheduleTask }
func (modelQualityScheduleHandler) Interval() time.Duration { return 15 * time.Second }
func (modelQualityScheduleHandler) NewPayload() any         { return nil }
func (modelQualityScheduleHandler) Enabled() bool {
	cfg, _, err := model.GetQualitySettings()
	return err == nil && (cfg.Enabled || cfg.RetentionEnabled)
}
func (modelQualityScheduleHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := ctx.Err()
	if err == nil {
		err = model.ScheduleQualityRuns(time.Now())
	}
	if err == nil && ctx.Err() == nil {
		err = model.CleanupQualityRetention(time.Now())
	}
	if err == nil && ctx.Err() == nil && model.HasPendingQualityWork() {
		_, _, err = service.EnqueueSystemTask(model.ModelQualityExecuteTask, nil)
	}
	finishModelQualityTask(task, runnerID, err)
}

type modelQualityExecuteHandler struct{}

func (modelQualityExecuteHandler) Type() string            { return model.ModelQualityExecuteTask }
func (modelQualityExecuteHandler) Interval() time.Duration { return 15 * time.Second }
func (modelQualityExecuteHandler) NewPayload() any         { return nil }
func (modelQualityExecuteHandler) Enabled() bool           { return model.HasPendingQualityWork() }
func (modelQualityExecuteHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := runModelQualityQueue(ctx, task.TaskID, qualityinspect.RunSample)
	finishModelQualityTask(task, runnerID, err)
}

func finishModelQualityTask(task *model.SystemTask, runnerID string, err error) {
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
		err = errors.New("model quality task failed")
	}
	finishSystemTaskHandler(task, runnerID, status, nil, err)
}

type qualitySampleOutcome struct {
	id     int64
	result model.QualityResult
}

type qualityInFlight struct {
	sample *model.ModelQualitySample
	config model.QualityConfig
	cancel context.CancelFunc
}

func runModelQualityQueue(ctx context.Context, executorID string, execute func(context.Context, model.QualityConfig, int) model.QualityResult) error {
	if err := model.RecoverQualityWork(executorID, time.Now()); err != nil {
		return err
	}
	results := make(chan qualitySampleOutcome, 2)
	active := make(map[int64]qualityInFlight)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var firstErr error
	stopped := false
	done := ctx.Done()
	for {
		if ctx.Err() != nil && !stopped {
			firstErr, stopped, done = ctx.Err(), true, nil
			for _, work := range active {
				work.cancel()
			}
		}
		settings, _, err := model.GetQualitySettings()
		if err != nil || !settings.Enabled {
			stopped = true
			if err != nil && firstErr == nil {
				firstErr = err
			}
			for _, work := range active {
				work.cancel()
			}
		}
		for !stopped && len(active) < settings.Concurrency {
			excluded := make([]int, 0, len(active))
			for _, work := range active {
				if work.sample.TargetChannelID > 0 {
					excluded = append(excluded, work.sample.TargetChannelID)
				}
			}
			sample, cfg, err := model.ClaimQualitySample(executorID, excluded, time.Now())
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			if err != nil {
				firstErr, stopped = err, true
				for _, work := range active {
					work.cancel()
				}
				break
			}
			requestCtx, cancel := context.WithCancel(ctx)
			active[sample.ID] = qualityInFlight{sample: sample, config: cfg, cancel: cancel}
			go func() {
				outcome := qualitySampleOutcome{id: sample.ID, result: model.QualityResult{Status: "interrupted", ErrorCode: "executor_interrupted"}}
				defer func() {
					if recover() != nil {
						outcome.result = model.QualityResult{Status: "interrupted", ErrorCode: "executor_interrupted"}
					}
					results <- outcome
				}()
				outcome.result = execute(requestCtx, cfg, sample.TargetChannelID)
			}()
		}
		if len(active) == 0 {
			return firstErr
		}
		select {
		case <-done:
			firstErr, stopped, done = ctx.Err(), true, nil
			for _, work := range active {
				work.cancel()
			}
		case outcome := <-results:
			work := active[outcome.id]
			work.cancel()
			delete(active, outcome.id)
			if ctx.Err() != nil {
				outcome.result.Status, outcome.result.ErrorCode = "interrupted", "executor_interrupted"
			}
			if err := model.FinishQualitySample(outcome.id, executorID, outcome.result, time.Now()); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				stopped = true
				for _, work := range active {
					work.cancel()
				}
			}
		case <-ticker.C:
			for _, work := range active {
				allowed, err := model.QualitySampleCanContinue(work.sample.ID, executorID, time.Now())
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					stopped = true
				}
				if allowed && err == nil {
					_, err = qualityinspect.ValidateRuntime(work.config, work.sample.TargetChannelID)
				}
				if !allowed || err != nil {
					work.cancel()
				}
			}
		}
	}
}
