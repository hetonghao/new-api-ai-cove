package service

import (
	"context"
	"sync"

	"github.com/QuantumNous/new-api/model"
)

const (
	RiskObservationErrorQueueFull = "queue_full"
	RiskObservationErrorShutdown  = "service_shutdown"
)

type RiskObservationJob struct {
	RequestID             string
	ChannelID             int
	ChannelName           string
	UserID                int
	TokenID               int
	Model                 string
	Path                  string
	Text                  string
	ProviderID            int
	ProviderIDs           []int
	ReviewMode            model.RiskReviewMode
	ActionMode            model.RiskActionMode
	NonBlockingCategories []string
}

type riskObservationQueueItemKind uint8

type RiskObservationEnqueueOutcome uint8

const (
	riskObservationQueueItemJob riskObservationQueueItemKind = iota
	riskObservationQueueItemEvent
)

const (
	RiskObservationEnqueueQueued RiskObservationEnqueueOutcome = iota
	RiskObservationEnqueueFallbackRetained
	RiskObservationEnqueueDirectRecordRequired
)

type RiskObservationEnqueueResult struct {
	Outcome   RiskObservationEnqueueOutcome
	ErrorCode string
}

type riskObservationQueueItem struct {
	kind  riskObservationQueueItemKind
	job   RiskObservationJob
	event RiskObservationEvent
}

type riskObservationDegradation struct {
	item riskObservationQueueItem
	code string
}

type RiskObservationQueueConfig struct {
	Capacity int
	Process  func(context.Context, RiskObservationJob)
	Record   func(context.Context, RiskObservationEvent)
	Degrade  func(context.Context, RiskObservationJob, string)
}

type RiskObservationQueue struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	items               chan riskObservationQueueItem
	degradations        chan riskObservationDegradation
	degradationSpillway chan riskObservationDegradation
	process             func(context.Context, RiskObservationJob)
	record              func(context.Context, RiskObservationEvent)
	degrade             func(context.Context, RiskObservationJob, string)
	done                chan struct{}
	mu                  sync.RWMutex
	closed              bool
	closeOnce           sync.Once
}

func NewRiskObservationQueue(ctx context.Context, config RiskObservationQueueConfig) *RiskObservationQueue {
	if config.Capacity < 1 {
		config.Capacity = 1
	}
	queueCtx, cancel := context.WithCancel(ctx)
	queue := &RiskObservationQueue{
		ctx:                 queueCtx,
		cancel:              cancel,
		items:               make(chan riskObservationQueueItem, config.Capacity),
		degradations:        make(chan riskObservationDegradation, config.Capacity),
		degradationSpillway: make(chan riskObservationDegradation, config.Capacity),
		process:             config.Process,
		record:              config.Record,
		degrade:             config.Degrade,
		done:                make(chan struct{}),
	}
	go queue.run()
	return queue
}

func (queue *RiskObservationQueue) Enqueue(job RiskObservationJob) RiskObservationEnqueueResult {
	return queue.enqueue(riskObservationQueueItem{kind: riskObservationQueueItemJob, job: job})
}

func (queue *RiskObservationQueue) EnqueueEvent(event RiskObservationEvent) RiskObservationEnqueueResult {
	return queue.enqueue(riskObservationQueueItem{kind: riskObservationQueueItemEvent, event: event})
}

func (queue *RiskObservationQueue) enqueue(item riskObservationQueueItem) RiskObservationEnqueueResult {
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	errorCode := ""
	switch item.kind {
	case riskObservationQueueItemJob:
		if queue.closed {
			errorCode = RiskObservationErrorShutdown
		} else {
			errorCode = RiskObservationErrorQueueFull
		}
	case riskObservationQueueItemEvent:
	}
	if queue.closed {
		return RiskObservationEnqueueResult{
			Outcome: RiskObservationEnqueueDirectRecordRequired, ErrorCode: errorCode,
		}
	}
	select {
	case queue.items <- item:
		return RiskObservationEnqueueResult{Outcome: RiskObservationEnqueueQueued}
	default:
		degradation := riskObservationDegradation{item: item, code: RiskObservationErrorQueueFull}
		select {
		case queue.degradations <- degradation:
			return RiskObservationEnqueueResult{
				Outcome: RiskObservationEnqueueFallbackRetained, ErrorCode: errorCode,
			}
		default:
			select {
			case queue.degradationSpillway <- degradation:
				return RiskObservationEnqueueResult{
					Outcome: RiskObservationEnqueueFallbackRetained, ErrorCode: errorCode,
				}
			default:
				return RiskObservationEnqueueResult{
					Outcome: RiskObservationEnqueueDirectRecordRequired, ErrorCode: errorCode,
				}
			}
		}
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
			queue.handleDegradation(degradation)
		case degradation := <-queue.degradationSpillway:
			queue.handleDegradation(degradation)
		case item := <-queue.items:
			queue.handleItem(queue.ctx, item)
		}
	}
}

func (queue *RiskObservationQueue) handleItem(ctx context.Context, item riskObservationQueueItem) {
	switch item.kind {
	case riskObservationQueueItemJob:
		if queue.process != nil {
			queue.process(ctx, item.job)
		}
	case riskObservationQueueItemEvent:
		if queue.record != nil {
			queue.record(ctx, item.event)
		}
	}
}

func (queue *RiskObservationQueue) handleDegradation(degradation riskObservationDegradation) {
	ctx := context.WithoutCancel(queue.ctx)
	if degradation.item.kind == riskObservationQueueItemEvent {
		queue.handleItem(ctx, degradation.item)
		return
	}
	if queue.degrade != nil {
		queue.degrade(ctx, degradation.item.job, degradation.code)
	}
}

func (queue *RiskObservationQueue) drain() {
	for {
		select {
		case degradation := <-queue.degradations:
			queue.handleDegradation(degradation)
		case degradation := <-queue.degradationSpillway:
			queue.handleDegradation(degradation)
		case item := <-queue.items:
			if item.kind == riskObservationQueueItemEvent {
				queue.handleItem(context.WithoutCancel(queue.ctx), item)
			} else if queue.degrade != nil {
				queue.degrade(context.WithoutCancel(queue.ctx), item.job, RiskObservationErrorShutdown)
			}
		default:
			return
		}
	}
}
