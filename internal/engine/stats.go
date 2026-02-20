package engine

import (
	"time"

	"gelato/internal/server"
	"gelato/internal/store"
)

type statsAggregator struct {
	queue []server.EngineStatsUpdate
	tick  *time.Ticker
}

func newStatsAggregator(capacity int) *statsAggregator {
	if capacity <= 0 {
		capacity = 128
	}
	return &statsAggregator{queue: make([]server.EngineStatsUpdate, 0, capacity), tick: time.NewTicker(100 * time.Millisecond)}
}

func (s *statsAggregator) Add(update server.EngineStatsUpdate) {
	s.queue = append(s.queue, update)
	if len(s.queue) >= cap(s.queue) {
		s.tick.Reset(10 * time.Millisecond)
	}
}

func (s *statsAggregator) Drain() []server.EngineStatsUpdate {
	updates := s.queue
	s.queue = make([]server.EngineStatsUpdate, 0, cap(updates))
	s.tick.Reset(100 * time.Millisecond)
	return updates
}

func (s *statsAggregator) Tick() <-chan time.Time {
	return s.tick.C
}

func (e *Engine) updateDropStats(now time.Time) {
	current := e.dropped
	if current > e.lastDroppedTotal {
		delta := current - e.lastDroppedTotal
		e.dropsPerMin.Add(delta, now)
		e.lastDroppedTotal = current
	}
}

func (e *Engine) flushStats(now time.Time) {
	updates := e.statAgg.Drain()
	for _, update := range updates {
		if update.Dropped > 0 {
			e.dropped += update.Dropped
			e.dropsPerMin.Add(update.Dropped, now)
			if update.ListenerID != "" {
				e.listenerDrops[update.ListenerID] += update.Dropped
				e.ensureListenerDropRate(update.ListenerID).Add(update.Dropped, now)
			}
			if update.SourceKey != "" {
				e.sourceDrops[update.SourceKey] += update.Dropped
				e.ensureSourceDropRate(update.SourceKey).Add(update.Dropped, now)
			}
			if update.Shard >= 0 && update.Shard < len(e.shardDrops) {
				e.shardDrops[update.Shard] += update.Dropped
			}
		}
		if update.Errors > 0 {
			e.errors += update.Errors
			e.errorsPerMin.Add(update.Errors, now)
			if update.ListenerID != "" {
				e.listenerErrors[update.ListenerID] += update.Errors
			}
			if update.SourceKey != "" {
				e.sourceErrors[update.SourceKey] += update.Errors
			}
		}
	}
}

func (e *Engine) ensureSourceErrorRate(key string) *store.RollingCounter {
	counter := e.sourceErrorRates[key]
	if counter == nil {
		counter = store.NewRollingCounter(30, time.Minute, time.Now())
		e.sourceErrorRates[key] = counter
	}
	return counter
}

func (e *Engine) ensureListenerErrorRate(id string) *store.RollingCounter {
	counter := e.listenerErrorRates[id]
	if counter == nil {
		counter = store.NewRollingCounter(30, time.Minute, time.Now())
		e.listenerErrorRates[id] = counter
	}
	return counter
}

func (e *Engine) ensureListenerDropRate(id string) *store.RollingCounter {
	counter := e.listenerDropRates[id]
	if counter == nil {
		counter = store.NewRollingCounter(30, time.Minute, time.Now())
		e.listenerDropRates[id] = counter
	}
	return counter
}

func (e *Engine) ensureSourceDropRate(key string) *store.RollingCounter {
	counter := e.sourceDropRates[key]
	if counter == nil {
		counter = store.NewRollingCounter(30, time.Minute, time.Now())
		e.sourceDropRates[key] = counter
	}
	return counter
}

func (e *Engine) buildSourceErrorRates(now time.Time) map[string][]uint64 {
	out := make(map[string][]uint64, len(e.sourceErrorRates))
	for key, counter := range e.sourceErrorRates {
		out[key] = counter.Snapshot(now)
	}
	return out
}

func (e *Engine) buildListenerErrorRates(now time.Time) map[string][]uint64 {
	out := make(map[string][]uint64, len(e.listenerErrorRates))
	for key, counter := range e.listenerErrorRates {
		out[key] = counter.Snapshot(now)
	}
	return out
}

func (e *Engine) buildListenerDropRates(now time.Time) map[string][]uint64 {
	out := make(map[string][]uint64, len(e.listenerDropRates))
	for key, counter := range e.listenerDropRates {
		out[key] = counter.Snapshot(now)
	}
	return out
}

func (e *Engine) buildSourceDropRates(now time.Time) map[string][]uint64 {
	out := make(map[string][]uint64, len(e.sourceDropRates))
	for key, counter := range e.sourceDropRates {
		out[key] = counter.Snapshot(now)
	}
	return out
}
