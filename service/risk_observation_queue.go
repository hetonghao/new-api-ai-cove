package service

import (
	"context"
	"sync"
)

const (
	RiskObservationErrorQueueFull = "queue_full"
	RiskObservationErrorShutdown  = "service_shutdown"
)

type RiskObservationJob struct {
	RequestID   string
	ChannelID   int
	ChannelName string
	UserID      int
	Text        string
}

type riskObservationDegradation struct {
	job  RiskObservationJob
	code string
}

type RiskObservationQueueConfig struct {
	Capacity int
	Process  func(context.Context, RiskObservationJob)
	Degrade  func(context.Context, RiskObservationJob, string)
}

type RiskObservationQueue struct {
	ctx          context.Context
	cancel       context.CancelFunc
	jobs         chan RiskObservationJob
	degradations chan riskObservationDegradation
	process      func(context.Context, RiskObservationJob)
	degrade      func(context.Context, RiskObservationJob, string)
	done         chan struct{}
	mu           sync.RWMutex
	closed       bool
	closeOnce    sync.Once
}

func NewRiskObservationQueue(ctx context.Context, config RiskObservationQueueConfig) *RiskObservationQueue {
	if config.Capacity < 1 {
		config.Capacity = 1
	}
	queueCtx, cancel := context.WithCancel(ctx)
	queue := &RiskObservationQueue{
		ctx:          queueCtx,
		cancel:       cancel,
		jobs:         make(chan RiskObservationJob, config.Capacity),
		degradations: make(chan riskObservationDegradation, config.Capacity),
		process:      config.Process,
		degrade:      config.Degrade,
		done:         make(chan struct{}),
	}
	go queue.run()
	return queue
}

func (queue *RiskObservationQueue) Enqueue(job RiskObservationJob) bool {
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	if queue.closed {
		return false
	}
	select {
	case queue.jobs <- job:
		return true
	default:
		select {
		case queue.degradations <- riskObservationDegradation{job: job, code: RiskObservationErrorQueueFull}:
		default:
		}
		return false
	}
}

func (queue *RiskObservationQueue) Close(ctx context.Context) {
	queue.closeOnce.Do(func() {
		queue.mu.Lock()
		queue.closed = true
		queue.cancel()
		queue.mu.Unlock()
	})
	select {
	case <-queue.done:
	case <-ctx.Done():
	}
}

func (queue *RiskObservationQueue) run() {
	defer close(queue.done)
	for {
		if queue.ctx.Err() != nil {
			queue.drain()
			return
		}
		select {
		case <-queue.ctx.Done():
			queue.drain()
			return
		case degradation := <-queue.degradations:
			queue.degrade(context.WithoutCancel(queue.ctx), degradation.job, degradation.code)
		case job := <-queue.jobs:
			queue.process(queue.ctx, job)
		}
	}
}

func (queue *RiskObservationQueue) drain() {
	for {
		select {
		case degradation := <-queue.degradations:
			queue.degrade(context.WithoutCancel(queue.ctx), degradation.job, degradation.code)
		case job := <-queue.jobs:
			queue.degrade(context.WithoutCancel(queue.ctx), job, RiskObservationErrorShutdown)
		default:
			return
		}
	}
}
