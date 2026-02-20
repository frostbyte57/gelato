package engine

import (
	"context"
	"time"

	"gelato/internal/config"
	"gelato/internal/model"
	"gelato/internal/server"
	"gelato/internal/store"
)

type Engine struct {
	limits config.Limits

	uiCmdCh    chan Command
	snapReqCh  chan SnapshotRequest
	snapRespCh chan Snapshot
	eventCh    []chan model.LogEvent
	ingestCh   chan model.LogEvent

	listeners          map[string]model.Listener
	sources            map[string]*model.Source
	globalBuf          *store.RingBuffer
	perSource          map[string]*store.RingBuffer
	filters            Filters
	listenerDrops      map[string]uint64
	listenerErrors     map[string]uint64
	sourceDrops        map[string]uint64
	sourceErrors       map[string]uint64
	sourceErrorRates   map[string]*store.RollingCounter
	sourceDropRates    map[string]*store.RollingCounter
	listenerErrorRates map[string]*store.RollingCounter
	listenerDropRates  map[string]*store.RollingCounter
	overflowSources    uint64
	overflowBuffer     *store.RingBuffer
	shardDrops         []uint64
	filterCache        filterCache

	dropped uint64
	errors  uint64

	logsPerSec   *store.RollingCounter
	errorsPerMin *store.RollingCounter
	dropsPerMin  *store.RollingCounter

	asyncIngest    bool
	pendingLogs    uint64
	drainBatchSize int

	lastID           uint64
	lastDroppedTotal uint64
	dropUpdates      chan server.EngineStatsUpdate
	statAgg          *statsAggregator

	startListenerFn func(ctx context.Context, listener model.Listener) (string, error)
	stopListenerFn  func(ctx context.Context, id string) error
	statsFn         func() map[string]ListenerStats
	sourceStatsFn   func() map[string]SourceStats
}

func New(limits config.Limits) *Engine {
	drainBatch := limits.DrainBatchSize
	if drainBatch <= 0 {
		drainBatch = 256
	}

	engine := &Engine{
		limits:             limits,
		uiCmdCh:            make(chan Command, 64),
		snapReqCh:          make(chan SnapshotRequest, 16),
		snapRespCh:         make(chan Snapshot, 16),
		eventCh:            make([]chan model.LogEvent, limits.ShardCount),
		listeners:          make(map[string]model.Listener),
		sources:            make(map[string]*model.Source),
		perSource:          make(map[string]*store.RingBuffer),
		listenerDrops:      make(map[string]uint64),
		listenerErrors:     make(map[string]uint64),
		sourceDrops:        make(map[string]uint64),
		sourceErrors:       make(map[string]uint64),
		sourceErrorRates:   make(map[string]*store.RollingCounter),
		sourceDropRates:    make(map[string]*store.RollingCounter),
		listenerErrorRates: make(map[string]*store.RollingCounter),
		listenerDropRates:  make(map[string]*store.RollingCounter),
		overflowBuffer:     store.NewRingBuffer(limits.PerSourceBufferLines),
		shardDrops:         make([]uint64, limits.ShardCount),
		globalBuf:          store.NewRingBuffer(limits.GlobalBufferLines),
		filterCache:        filterCache{},
		filters:            Filters{LevelMask: model.LevelMaskAll},
		logsPerSec:         store.NewRollingCounter(60, time.Second, time.Now()),
		errorsPerMin:       store.NewRollingCounter(30, time.Minute, time.Now()),
		dropsPerMin:        store.NewRollingCounter(30, time.Minute, time.Now()),
		lastDroppedTotal:   0,
		dropUpdates:        make(chan server.EngineStatsUpdate, 1024),
		statAgg:            newStatsAggregator(256),
		asyncIngest:        limits.AsyncIngest,
		drainBatchSize:     drainBatch,
	}

	for i := range engine.eventCh {
		engine.eventCh[i] = make(chan model.LogEvent, limits.QueueSizePerShard)
	}

	if engine.asyncIngest {
		queueSize := limits.IngestQueueSize
		if queueSize <= 0 {
			queueSize = limits.ShardCount * 512
		}
		engine.ingestCh = make(chan model.LogEvent, queueSize)
	}

	return engine
}

func (e *Engine) AttachListenerHandlers(
	startFn func(ctx context.Context, listener model.Listener) (string, error),
	stopFn func(ctx context.Context, id string) error,
	statsFn func() map[string]ListenerStats,
	sourceStatsFn func() map[string]SourceStats,
) {
	e.startListenerFn = startFn
	e.stopListenerFn = stopFn
	e.statsFn = statsFn
	e.sourceStatsFn = sourceStatsFn
}

func (e *Engine) UICmdCh() chan<- Command {
	return e.uiCmdCh
}

func (e *Engine) SnapshotReqCh() chan<- SnapshotRequest {
	return e.snapReqCh
}

func (e *Engine) SnapshotRespCh() <-chan Snapshot {
	return e.snapRespCh
}

func (e *Engine) EventShards() []chan<- model.LogEvent {
	out := make([]chan<- model.LogEvent, len(e.eventCh))
	for i, ch := range e.eventCh {
		out[i] = ch
	}
	return out
}

func (e *Engine) DropSink() func(update server.EngineStatsUpdate) {
	return func(update server.EngineStatsUpdate) {
		if update.Dropped == 0 {
			return
		}
		select {
		case e.dropUpdates <- update:
		default:
		}
	}
}

func (e *Engine) Run(ctx context.Context) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	if e.asyncIngest {
		e.startAsyncIngest(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt := <-e.ingestCh:
			e.consumeIngestEvent(evt)
			e.drainIngestBurst()
		case cmd := <-e.uiCmdCh:
			if err := e.handleCommand(cmd); err != nil {
				e.errors++
			}
		case req := <-e.snapReqCh:
			snap := e.buildSnapshot(req)
			e.snapRespCh <- snap
		case drop := <-e.dropUpdates:
			e.statAgg.Add(drop)
		case <-e.statAgg.Tick():
			e.flushStats(time.Now())
		case <-tick.C:
			now := time.Now()
			if e.asyncIngest {
				e.flushPendingLogs(now)
			} else {
				processed := e.drainEvents(now, e.drainBatchSize)
				if processed > 0 {
					e.logsPerSec.Add(processed, now)
				}
			}
		}
	}
}
