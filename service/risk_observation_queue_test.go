package service

import (
	"context"
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
