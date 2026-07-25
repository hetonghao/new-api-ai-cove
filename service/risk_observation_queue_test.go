package service

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "running"}))
	<-started
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "queued"}))

	// When
	accepted := queue.Enqueue(RiskObservationJob{RequestID: "overflow"})

	// Then
	require.False(t, accepted)
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
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "running"}))
	<-started
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "queued"}))
	require.False(t, queue.Enqueue(RiskObservationJob{RequestID: "overflow-primary"}))

	// When
	accepted := queue.Enqueue(RiskObservationJob{RequestID: "overflow-spillway"})
	close(release)
	queue.Close(context.Background())

	// Then
	require.False(t, accepted)
	degradationMu.Lock()
	gotRequestIDs := append([]string(nil), degradedRequestIDs...)
	degradationMu.Unlock()
	require.ElementsMatch(t, []string{"overflow-primary", "overflow-spillway"}, gotRequestIDs)
}

func TestRiskObservationQueue_counts_loss_when_bounded_degradation_spillway_is_full(t *testing.T) {
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
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "running"}))
	<-started
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "queued"}))
	require.False(t, queue.Enqueue(RiskObservationJob{RequestID: "overflow-primary"}))
	require.False(t, queue.Enqueue(RiskObservationJob{RequestID: "overflow-spillway"}))

	// When
	accepted := queue.Enqueue(RiskObservationJob{RequestID: "overflow-beyond-ceiling"})
	lossCount := queue.DegradationLossCount()
	close(release)
	queue.Close(context.Background())

	// Then
	require.False(t, accepted)
	require.Equal(t, uint64(1), lossCount)
	close(degradedRequestIDs)
	gotRequestIDs := make([]string, 0, len(degradedRequestIDs))
	for requestID := range degradedRequestIDs {
		gotRequestIDs = append(gotRequestIDs, requestID)
	}
	require.ElementsMatch(t, []string{"overflow-primary", "overflow-spillway"}, gotRequestIDs)
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
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "running"}))
	<-started
	require.True(t, queue.Enqueue(RiskObservationJob{RequestID: "pending"}))

	// When
	queue.Close(context.Background())

	// Then
	require.Equal(t, "pending", (<-degraded).RequestID)
	require.False(t, queue.Enqueue(RiskObservationJob{RequestID: "after-close"}))
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
	accepted := queue.EnqueueEvent(event)

	// Then
	require.True(t, accepted)
	require.Equal(t, event, <-recorded)
	select {
	case job := <-processed:
		t.Fatalf("precomputed event was processed as a review job: %+v", job)
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
	require.True(t, queue.EnqueueEvent(RiskObservationEvent{RequestID: "slow"}))
	<-recording

	// When
	accepted := queue.EnqueueEvent(RiskObservationEvent{RequestID: "queued"})

	// Then
	require.True(t, accepted)
}
