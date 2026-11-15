package service

import (
	"context"
	"github.com/QuantumNous/new-api/model"
	"time"
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
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]int{"checked": checked, "failed": failed}, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() { RegisterSystemTaskHandler(upstreamSubscriptionQuotaHandler{}) }
