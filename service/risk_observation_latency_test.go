package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_does_not_wait_for_slow_sink_in_block_mode(t *testing.T) {
	tests := []struct {
		name        string
		status      RiskReviewStatus
		wantBlocked bool
	}{
		{name: "safe", status: RiskReviewSafe},
		{name: "unsafe", status: RiskReviewUnsafe, wantBlocked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			provider := createActiveRiskProvider(t, "https://example.com")
			providerID := provider.Id
			_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
				ProviderID:      &providerID,
				EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
			})
			require.NoError(t, err)
			recording := make(chan struct{})
			release := make(chan struct{})
			var recordingOnce sync.Once
			queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
				Capacity: 1,
				Record: func(context.Context, RiskObservationEvent) {
					recordingOnce.Do(func() { close(recording) })
					<-release
				},
			})
			t.Cleanup(func() {
				close(release)
				queue.Close(context.Background())
			})
			deps := riskObservationRelayDeps{
				executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
					return RiskModerationOutcome{Result: RiskReviewResult{Status: test.status}}, nil
				}),
				enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
					return queue.EnqueueEvent(event)
				},
			}
			decisions := make(chan RiskObservationRelayDecision, 1)

			// When
			go func() {
				decisions <- processRiskObservationForRelay(context.Background(), RiskObservationJob{
					RequestID: "slow-sink", ChannelName: "cpa-pro", Text: "current",
				}, deps)
			}()

			// Then
			select {
			case decision := <-decisions:
				require.Equal(t, test.wantBlocked, decision.Blocked)
			case <-recording:
				select {
				case decision := <-decisions:
					require.Equal(t, test.wantBlocked, decision.Blocked)
				case <-time.After(500 * time.Millisecond):
					t.Fatal("block decision waited for the observation sink")
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("block decision did not complete")
			}
		})
	}
}
