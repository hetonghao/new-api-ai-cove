package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type upstreamSubscriptionQuotaHandler struct{}

func (upstreamSubscriptionQuotaHandler) Type() string            { return "upstream_subscription_quota" }
func (upstreamSubscriptionQuotaHandler) Interval() time.Duration { return time.Minute }
func (upstreamSubscriptionQuotaHandler) NewPayload() any         { return nil }
func (upstreamSubscriptionQuotaHandler) Enabled() bool {
	ids, err := model.DueUpstreamSubscriptionIDs(time.Now().Unix())
	return err == nil && len(ids) > 0
}
func (upstreamSubscriptionQuotaHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	ids, err := model.DueUpstreamSubscriptionIDs(time.Now().Unix())
	report := NewUnthrottledSystemTaskProgressReporter(task, runnerID)
	if err == nil && ctx.Err() == nil && len(ids) > 0 {
		report(0, len(ids))
	}
	checked, failed := 0, 0
	for _, id := range ids {
		if err != nil || ctx.Err() != nil {
			break
		}
		var subscription *model.UpstreamSubscription
		subscription, err = RefreshUpstreamSubscription(ctx, id, false)
		if err == nil {
			checked++
			if subscription.ErrorCode != "" {
				failed++
			}
			if ctx.Err() == nil {
				report(checked, len(ids))
			}
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if len(ids) == 0 {
		report(0, 0)
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]int{"checked": checked, "failed": failed}, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() { RegisterSystemTaskHandler(upstreamSubscriptionQuotaHandler{}) }
