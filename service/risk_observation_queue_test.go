package service

import (
	"context"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestRiskObservationQueue_propagates_request_id_to_process_context(t *testing.T) {
	// Given
	requestIDs := make(chan string, 1)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(ctx context.Context, _ RiskObservationJob) {
			requestID, _ := ctx.Value(common.RequestIdKey).(string)
			requestIDs <- requestID
		},
	})
	t.Cleanup(func() { queue.Close(context.Background()) })

	// When
	result := queue.Enqueue(RiskObservationJob{RequestID: "req-queue-timeout"})

	// Then
	require.Equal(t, RiskObservationEnqueueQueued, result.Outcome)
	require.Equal(t, "req-queue-timeout", <-requestIDs)
}

func TestRiskObservationQueue_returns_immediately_when_full(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	degraded := make(chan string, 1)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(ctx context.Context, job RiskObservationJob) {
			if job.RequestID == "running" {
				close(started)
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
		},
		Degrade: func(_ context.Context, _ RiskObservationJob, code string) { degraded <- code },
	})
	t.Cleanup(func() {
		queue.Close(context.Background())
	})
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "running"}).Outcome)
	<-started
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "queued"}).Outcome)

	// When
	result := queue.Enqueue(RiskObservationJob{RequestID: "overflow"})

	// Then
	require.Equal(t, RiskObservationEnqueueFallbackRetained, result.Outcome)
	require.Equal(t, RiskObservationErrorQueueFull, result.ErrorCode)
	close(release)
	require.Equal(t, RiskObservationErrorQueueFull, <-degraded)
}

func TestRiskObservationQueue_preserves_queue_full_degradation_when_primary_degradation_buffer_is_full(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	var degradationMu sync.Mutex
	degradedRequestIDs := make([]string, 0, 2)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(_ context.Context, job RiskObservationJob) {
			if job.RequestID == "running" {
				close(started)
			}
			<-release
		},
		Degrade: func(_ context.Context, job RiskObservationJob, code string) {
			if code != RiskObservationErrorQueueFull {
				return
			}
			degradationMu.Lock()
			degradedRequestIDs = append(degradedRequestIDs, job.RequestID)
			degradationMu.Unlock()
		},
	})
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "running"}).Outcome)
	<-started
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "queued"}).Outcome)
	require.Equal(t, RiskObservationEnqueueFallbackRetained, queue.Enqueue(RiskObservationJob{RequestID: "overflow-primary"}).Outcome)

	// When
	result := queue.Enqueue(RiskObservationJob{RequestID: "overflow-spillway"})
	close(release)
	queue.Close(context.Background())

	// Then
	require.Equal(t, RiskObservationEnqueueFallbackRetained, result.Outcome)
	require.Equal(t, RiskObservationErrorQueueFull, result.ErrorCode)
	degradationMu.Lock()
	gotRequestIDs := append([]string(nil), degradedRequestIDs...)
	degradationMu.Unlock()
	require.ElementsMatch(t, []string{"overflow-primary", "overflow-spillway"}, gotRequestIDs)
}

func TestRiskObservationQueue_requires_direct_queue_full_record_when_bounded_fallback_is_full(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	degradedRequestIDs := make(chan string, 2)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(_ context.Context, job RiskObservationJob) {
			if job.RequestID == "running" {
				close(started)
			}
			<-release
		},
		Degrade: func(_ context.Context, job RiskObservationJob, code string) {
			if code == RiskObservationErrorQueueFull {
				degradedRequestIDs <- job.RequestID
			}
		},
	})
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "running"}).Outcome)
	<-started
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "queued"}).Outcome)
	require.Equal(t, RiskObservationEnqueueFallbackRetained, queue.Enqueue(RiskObservationJob{RequestID: "overflow-primary"}).Outcome)
	require.Equal(t, RiskObservationEnqueueFallbackRetained, queue.Enqueue(RiskObservationJob{RequestID: "overflow-spillway"}).Outcome)

	// When
	result := queue.Enqueue(RiskObservationJob{RequestID: "overflow-direct"})
	close(release)
	queue.Close(context.Background())

	// Then
	require.Equal(t, RiskObservationEnqueueDirectRecordRequired, result.Outcome)
	require.Equal(t, RiskObservationErrorQueueFull, result.ErrorCode)
	close(degradedRequestIDs)
	gotRequestIDs := make([]string, 0, len(degradedRequestIDs))
	for requestID := range degradedRequestIDs {
		gotRequestIDs = append(gotRequestIDs, requestID)
	}
	require.ElementsMatch(t, []string{"overflow-primary", "overflow-spillway"}, gotRequestIDs)
}

func TestRiskObservationQueue_requires_direct_shutdown_record_when_closed(t *testing.T) {
	// Given
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{Capacity: 1})
	queue.Close(context.Background())

	// When
	result := queue.Enqueue(RiskObservationJob{RequestID: "after-close"})

	// Then
	require.Equal(t, RiskObservationEnqueueDirectRecordRequired, result.Outcome)
	require.Equal(t, RiskObservationErrorShutdown, result.ErrorCode)
}

func TestRiskObservationQueue_requires_direct_original_event_record_when_closed(t *testing.T) {
	// Given
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{Capacity: 1})
	queue.Close(context.Background())

	// When
	result := queue.EnqueueEvent(RiskObservationEvent{RequestID: "event-after-close"})

	// Then
	require.Equal(t, RiskObservationEnqueueDirectRecordRequired, result.Outcome)
	require.Empty(t, result.ErrorCode)
}

func TestRecordRiskObservationDegradationDirect_uses_live_bounded_context_after_cancel(t *testing.T) {
	// Given
	var recordedCtxErr error
	var recordedCtxHasDeadline bool
	var recordedEvent RiskObservationEvent
	SetRiskObservationSink(riskObservationSinkFunc(func(ctx context.Context, event RiskObservationEvent) error {
		recordedCtxErr = ctx.Err()
		_, recordedCtxHasDeadline = ctx.Deadline()
		recordedEvent = event
		return nil
	}))
	t.Cleanup(func() { SetRiskObservationSink(nil) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	RecordRiskObservationDegradationDirect(ctx, RiskObservationJob{RequestID: "direct"}, RiskObservationErrorQueueFull)

	// Then
	require.NoError(t, recordedCtxErr)
	require.True(t, recordedCtxHasDeadline)
	require.Equal(t, "direct", recordedEvent.RequestID)
	require.Equal(t, RiskObservationErrorQueueFull, recordedEvent.ErrorCode)
}

func TestProcessRiskObservation_records_shutdown_with_live_bounded_context(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	var recordedCtxErr error
	var recordedCtxHasDeadline bool
	var recordedEvent RiskObservationEvent
	SetRiskObservationSink(riskObservationSinkFunc(func(ctx context.Context, event RiskObservationEvent) error {
		recordedCtxErr = ctx.Err()
		_, recordedCtxHasDeadline = ctx.Deadline()
		recordedEvent = event
		return nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	processRiskObservation(ctx, RiskObservationJob{
		RequestID: "running-at-shutdown", ProviderID: provider.Id,
		ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionObserve,
		Text: "current",
	})

	// Then
	require.NoError(t, recordedCtxErr)
	require.True(t, recordedCtxHasDeadline)
	require.Equal(t, RiskObservationError, recordedEvent.Result)
	require.Equal(t, RiskObservationErrorShutdown, recordedEvent.ErrorCode)
}

func TestRiskObservationQueue_degrades_pending_jobs_on_close(t *testing.T) {
	// Given
	started := make(chan struct{})
	degraded := make(chan RiskObservationJob, 1)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(ctx context.Context, _ RiskObservationJob) {
			close(started)
			<-ctx.Done()
		},
		Degrade: func(_ context.Context, job RiskObservationJob, code string) {
			if code == RiskObservationErrorShutdown {
				degraded <- job
			}
		},
	})
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "running"}).Outcome)
	<-started
	require.Equal(t, RiskObservationEnqueueQueued, queue.Enqueue(RiskObservationJob{RequestID: "pending"}).Outcome)

	// When
	queue.Close(context.Background())

	// Then
	require.Equal(t, "pending", (<-degraded).RequestID)
	require.Equal(t, RiskObservationEnqueueDirectRecordRequired, queue.Enqueue(RiskObservationJob{RequestID: "after-close"}).Outcome)
}

func TestRiskObservationQueue_records_precomputed_event_without_processing_job(t *testing.T) {
	// Given
	processed := make(chan RiskObservationJob, 1)
	recorded := make(chan RiskObservationEvent, 1)
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Process: func(_ context.Context, job RiskObservationJob) {
			processed <- job
		},
		Record: func(_ context.Context, event RiskObservationEvent) {
			recorded <- event
		},
	})
	t.Cleanup(func() {
		queue.Close(context.Background())
	})
	event := RiskObservationEvent{RequestID: "precomputed", Result: RiskObservationUnsafe}

	// When
	result := queue.EnqueueEvent(event)

	// Then
	require.Equal(t, RiskObservationEnqueueQueued, result.Outcome)
	require.Equal(t, event, <-recorded)
	select {
	case job := <-processed:
		require.Failf(t, "precomputed event was processed as a review job", "job: %+v", job)
	default:
	}
}

func TestRiskObservationQueue_returns_immediately_when_precomputed_event_sink_is_slow(t *testing.T) {
	// Given
	recording := make(chan struct{})
	release := make(chan struct{})
	var recordingOnce sync.Once
	queue := NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: 1,
		Record: func(_ context.Context, _ RiskObservationEvent) {
			recordingOnce.Do(func() { close(recording) })
			<-release
		},
	})
	t.Cleanup(func() {
		close(release)
		queue.Close(context.Background())
	})
	require.Equal(t, RiskObservationEnqueueQueued, queue.EnqueueEvent(RiskObservationEvent{RequestID: "slow"}).Outcome)
	<-recording

	// When
	result := queue.EnqueueEvent(RiskObservationEvent{RequestID: "queued"})

	// Then
	require.Equal(t, RiskObservationEnqueueQueued, result.Outcome)
}
