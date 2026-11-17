package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withSystemTaskRegistry swaps the package registry for the given handlers for
// the duration of a test and restores the original registry afterward.
func withSystemTaskRegistry(t *testing.T, handlers ...SystemTaskHandler) {
	t.Helper()
	systemTaskHandlersMu.Lock()
	saved := systemTaskHandlers
	systemTaskHandlers = map[string]SystemTaskHandler{}
	for _, h := range handlers {
		systemTaskHandlers[h.Type()] = h
	}
	systemTaskHandlersMu.Unlock()
	t.Cleanup(func() {
		systemTaskHandlersMu.Lock()
		systemTaskHandlers = saved
		systemTaskHandlersMu.Unlock()
	})
}

type stubScheduledHandler struct {
	taskType string
	enabled  bool
	interval time.Duration
	onRun    func(ctx context.Context, task *model.SystemTask, runnerID string)
}

type stubSystemTaskRunResult struct {
	taskID   string
	taskType string
	err      error
}

func (h *stubScheduledHandler) Type() string { return h.taskType }

func (h *stubScheduledHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if h.onRun != nil {
		h.onRun(ctx, task, runnerID)
	}
}

func (h *stubScheduledHandler) Enabled() bool           { return h.enabled }
func (h *stubScheduledHandler) Interval() time.Duration { return h.interval }
func (h *stubScheduledHandler) NewPayload() any         { return nil }

func countSystemTasks(t *testing.T, taskType string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(&model.SystemTask{}).Where("type = ?", taskType).Count(&count).Error)
	return count
}

func TestSystemTaskSchedulerCreatesWhenDueAndDedups(t *testing.T) {
	truncate(t)

	handler := &stubScheduledHandler{taskType: "test_scheduled", enabled: true, interval: time.Minute}
	withSystemTaskRegistry(t, handler)

	runSystemTaskScheduler()
	require.Equal(t, int64(1), countSystemTasks(t, handler.taskType))

	// An active (pending) row already exists, so a second pass must not create
	// another row.
	runSystemTaskScheduler()
	require.Equal(t, int64(1), countSystemTasks(t, handler.taskType))

	// Finish the run; with a fresh updated_at the next run is not due yet.
	latest, err := model.GetLatestSystemTask(handler.taskType)
	require.NoError(t, err)
	require.NotNil(t, latest)
	_, claimed, err := model.ClaimSystemTask(latest.ID, handler.taskType, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, model.FinishSystemTask(latest.TaskID, "runner-a", model.SystemTaskStatusSucceeded, nil, ""))

	runSystemTaskScheduler()
	require.Equal(t, int64(1), countSystemTasks(t, handler.taskType))

	// Backdate the finished row beyond the interval -> the job becomes due again.
	require.NoError(t, model.DB.Model(&model.SystemTask{}).
		Where("task_id = ?", latest.TaskID).
		Update("updated_at", common.GetTimestamp()-120).Error)

	runSystemTaskScheduler()
	require.Equal(t, int64(2), countSystemTasks(t, handler.taskType))
}

func TestSystemTaskSchedulerSkipsDisabled(t *testing.T) {
	truncate(t)

	handler := &stubScheduledHandler{taskType: "test_disabled", enabled: false, interval: time.Minute}
	withSystemTaskRegistry(t, handler)

	runSystemTaskScheduler()
	assert.Equal(t, int64(0), countSystemTasks(t, handler.taskType))
}

func TestSystemTaskClaimPassDispatchesByType(t *testing.T) {
	truncate(t)

	ran := make(chan stubSystemTaskRunResult, 1)
	handler := &stubScheduledHandler{
		taskType: "test_dispatch",
		enabled:  true,
		interval: time.Minute,
		onRun: func(_ context.Context, task *model.SystemTask, runnerID string) {
			ran <- stubSystemTaskRunResult{
				taskType: task.Type,
				err:      model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""),
			}
		},
	}
	withSystemTaskRegistry(t, handler)

	_, err := model.CreateSystemTask(handler.taskType, nil, nil)
	require.NoError(t, err)

	runSystemTaskClaimPass("runner-dispatch")

	select {
	case got := <-ran:
		require.NoError(t, got.err)
		assert.Equal(t, handler.taskType, got.taskType)
	case <-time.After(2 * time.Second):
		t.Fatal("claimed task was not dispatched to its handler")
	}

	require.Eventually(t, func() bool {
		latest, err := model.GetLatestSystemTask(handler.taskType)
		return err == nil && latest != nil && latest.Status == model.SystemTaskStatusSucceeded
	}, 2*time.Second, 20*time.Millisecond)
}

func TestSystemTaskClaimPassDispatchesEarliestPendingByType(t *testing.T) {
	truncate(t)

	ran := make(chan stubSystemTaskRunResult, 2)
	handlerA := &stubScheduledHandler{
		taskType: "test_dispatch_a",
		enabled:  true,
		interval: time.Minute,
		onRun: func(_ context.Context, task *model.SystemTask, runnerID string) {
			ran <- stubSystemTaskRunResult{
				taskID: task.TaskID,
				err:    model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""),
			}
		},
	}
	handlerB := &stubScheduledHandler{
		taskType: "test_dispatch_b",
		enabled:  true,
		interval: time.Minute,
		onRun: func(_ context.Context, task *model.SystemTask, runnerID string) {
			ran <- stubSystemTaskRunResult{
				taskID: task.TaskID,
				err:    model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""),
			}
		},
	}
	withSystemTaskRegistry(t, handlerA, handlerB)

	firstA, err := model.CreateSystemTask(handlerA.taskType, nil, nil)
	require.NoError(t, err)
	secondTaskID, err := model.GenerateSystemTaskID()
	require.NoError(t, err)
	secondA := &model.SystemTask{
		TaskID: secondTaskID,
		Type:   handlerA.taskType,
		Status: model.SystemTaskStatusPending,
	}
	require.NoError(t, model.DB.Create(secondA).Error)
	firstB, err := model.CreateSystemTask(handlerB.taskType, nil, nil)
	require.NoError(t, err)

	runSystemTaskClaimPass("runner-dispatch")

	got := map[string]bool{}
	for range 2 {
		select {
		case result := <-ran:
			require.NoError(t, result.err)
			got[result.taskID] = true
		case <-time.After(2 * time.Second):
			t.Fatal("claimed tasks were not dispatched to their handlers")
		}
	}

	assert.True(t, got[firstA.TaskID])
	assert.True(t, got[firstB.TaskID])
	assert.False(t, got[secondA.TaskID])

	require.Eventually(t, func() bool {
		reloaded, err := model.GetSystemTaskByTaskID(secondA.TaskID)
		return err == nil && reloaded != nil && reloaded.Status == model.SystemTaskStatusPending
	}, 2*time.Second, 20*time.Millisecond)
}

func TestUpstreamSubscriptionQuotaTaskReportsProgress(t *testing.T) {
	for _, tc := range []struct {
		name      string
		count     int
		cancelled bool
	}{
		{name: "two subscriptions", count: 2},
		{name: "empty batch"},
		{name: "cancelled batch", count: 2, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			require.NoError(t, model.DB.AutoMigrate(&model.UpstreamSubscription{}))
			var ids []int
			t.Cleanup(func() {
				if len(ids) > 0 {
					require.NoError(t, model.DB.Where("id IN ?", ids).Delete(&model.UpstreamSubscription{}).Error)
				}
			})
			for range tc.count {
				subscription := &model.UpstreamSubscription{Name: "fixture", Platform: model.SubscriptionPlatformOpenCode, CredentialEncrypted: "invalid", SubscriptionInfoNextRefreshAt: time.Now().Add(time.Hour).Unix()}
				require.NoError(t, model.DB.Create(subscription).Error)
				ids = append(ids, subscription.ID)
			}
			handler := upstreamSubscriptionQuotaHandler{}
			task, err := model.CreateSystemTask(handler.Type(), nil, nil)
			require.NoError(t, err)
			claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "quota-runner", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}

			handler.Run(ctx, claimed, "quota-runner")
			stored, err := model.GetSystemTaskByTaskID(task.TaskID)
			require.NoError(t, err)
			require.NotNil(t, stored)
			var progress SystemTaskProgress
			require.NoError(t, stored.DecodeState(&progress))
			if tc.cancelled {
				assert.Equal(t, model.SystemTaskStatusFailed, stored.Status)
				assert.Less(t, progress.Progress, 100)
				return
			}
			assert.Equal(t, model.SystemTaskStatusSucceeded, stored.Status)
			assert.Equal(t, SystemTaskProgress{Total: tc.count, Processed: tc.count, Progress: 100}, progress)
			var result struct {
				Checked int `json:"checked"`
				Failed  int `json:"failed"`
			}
			require.NoError(t, common.UnmarshalJsonStr(stored.Result, &result))
			assert.Equal(t, tc.count, result.Checked)
			assert.Equal(t, tc.count, result.Failed)
		})
	}
}

func TestUpstreamSubscriptionQuotaTaskReportsWhileRunning(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "completed"
		if cancelled {
			name = "cancelled during refresh"
		}
		t.Run(name, func(t *testing.T) {
			truncate(t)
			require.NoError(t, model.DB.AutoMigrate(&model.UpstreamSubscription{}))
			previousSecret, previousClient := common.CryptoSecret, httpClient
			common.CryptoSecret = "subscription-progress-test"
			t.Cleanup(func() { common.CryptoSecret, httpClient = previousSecret, previousClient })
			credential, err := common.EncryptCredential("oc_sk_test")
			require.NoError(t, err)
			for range 2 {
				subscription := &model.UpstreamSubscription{Name: "fixture", Platform: model.SubscriptionPlatformOpenCode, CredentialEncrypted: credential, SubscriptionInfoNextRefreshAt: time.Now().Add(time.Hour).Unix()}
				require.NoError(t, model.DB.Create(subscription).Error)
				t.Cleanup(func() { require.NoError(t, model.DB.Delete(subscription).Error) })
			}
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			calls := 0
			httpClient = &http.Client{Transport: riskProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: request}, nil
				}
				close(started)
				select {
				case <-release:
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: request}, nil
				case <-request.Context().Done():
					return nil, request.Context().Err()
				}
			})}
			handler := upstreamSubscriptionQuotaHandler{}
			task, err := model.CreateSystemTask(handler.Type(), nil, nil)
			require.NoError(t, err)
			claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "quota-runner", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			go func() { defer close(done); handler.Run(ctx, claimed, "quota-runner") }()
			defer func() { cancel(); <-done }()

			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("subscription refresh did not start")
			}
			running, err := model.GetSystemTaskByTaskID(task.TaskID)
			require.NoError(t, err)
			require.Equal(t, model.SystemTaskStatusRunning, running.Status)
			var progress SystemTaskProgress
			require.NoError(t, running.DecodeState(&progress))
			assert.Equal(t, SystemTaskProgress{Total: 2, Processed: 1, Progress: 50}, progress)
			if cancelled {
				cancel()
			} else {
				close(release)
			}
			<-done
			finished, err := model.GetSystemTaskByTaskID(task.TaskID)
			require.NoError(t, err)
			require.NoError(t, finished.DecodeState(&progress))
			if cancelled {
				assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
				assert.Equal(t, SystemTaskProgress{Total: 2, Processed: 1, Progress: 50}, progress)
			} else {
				assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
				assert.Equal(t, SystemTaskProgress{Total: 2, Processed: 2, Progress: 100}, progress)
			}
		})
	}
}

func TestSystemTaskProgressReporterRetriesAfterWriteFailure(t *testing.T) {
	truncate(t)
	task, err := model.CreateSystemTask("progress_retry", nil, nil)
	require.NoError(t, err)
	_, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "runner", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	fail := true
	const callback = "test:progress_write_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if fail && tx.Statement.Table == "system_tasks" {
			fail = false
			tx.AddError(errors.New("temporary progress write failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })

	report := NewUnthrottledSystemTaskProgressReporter(task, "runner")
	report(0, 2)
	assert.False(t, fail)
	stored, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	var progress SystemTaskProgress
	require.NoError(t, stored.DecodeState(&progress))
	assert.Equal(t, SystemTaskProgress{Total: 2}, progress)
}

func TestSystemTaskProgressReporterDoesNotRetryLostLease(t *testing.T) {
	truncate(t)
	task, err := model.CreateSystemTask("progress_lost_lease", nil, nil)
	require.NoError(t, err)
	_, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "runner", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, model.DB.Model(&model.SystemTaskLock{}).Where("task_id = ?", task.TaskID).Update("locked_by", "other").Error)
	calls := 0
	const callback = "test:progress_lost_lease"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "system_tasks" {
			calls++
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })

	NewUnthrottledSystemTaskProgressReporter(task, "runner")(0, 2)
	assert.Equal(t, 1, calls)
}

func TestEnqueueSystemTaskReportsCreatedAndExistingActive(t *testing.T) {
	truncate(t)

	first, created, err := EnqueueSystemTask("test_enqueue", map[string]bool{"manual": true})
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, first)

	existing, created, err := EnqueueSystemTask("test_enqueue", nil)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, existing)
	assert.Equal(t, first.TaskID, existing.TaskID)

	_, claimed, err := model.ClaimSystemTask(first.ID, first.Type, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, model.FinishSystemTask(first.TaskID, "runner-a", model.SystemTaskStatusSucceeded, nil, ""))

	second, created, err := EnqueueSystemTask("test_enqueue", nil)
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, second)
	assert.NotEqual(t, first.TaskID, second.TaskID)
}
