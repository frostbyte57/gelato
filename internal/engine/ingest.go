package engine

import (
	"context"
	"hash/fnv"
	"time"

	"gelato/internal/model"
	"gelato/internal/store"
)

func (e *Engine) startAsyncIngest(ctx context.Context) {
	for i := range e.eventCh {
		ch := e.eventCh[i]
		go func(input <-chan model.LogEvent) {
			for {
				select {
				case <-ctx.Done():
					return
				case evt := <-input:
					select {
					case e.ingestCh <- evt:
					case <-ctx.Done():
						return
					}
				}
			}
		}(ch)
	}
}

func (e *Engine) consumeIngestEvent(evt model.LogEvent) {
	e.applyEvent(evt, time.Now())
	e.pendingLogs++
}

func (e *Engine) drainIngestBurst() {
	if e.ingestCh == nil {
		return
	}
	for i := 0; i < 32; i++ {
		select {
		case evt := <-e.ingestCh:
			e.consumeIngestEvent(evt)
		default:
			return
		}
	}
}

func (e *Engine) flushPendingLogs(now time.Time) {
	if e.pendingLogs == 0 {
		return
	}
	e.logsPerSec.Add(e.pendingLogs, now)
	e.pendingLogs = 0
}

func (e *Engine) AssignShard(sourceKey string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sourceKey))
	return int(h.Sum32()) % len(e.eventCh)
}

func (e *Engine) drainEvents(now time.Time, batchSize int) uint64 {
	if batchSize <= 0 {
		batchSize = 256
	}
	processed := uint64(0)
	maxExtra := batchSize * 4
	for _, ch := range e.eventCh {
		limit := batchSize
		if pending := len(ch); pending > limit {
			limit = pending
			if limit > maxExtra {
				limit = maxExtra
			}
		}
		for i := 0; i < limit; i++ {
			select {
			case evt := <-ch:
				e.applyEvent(evt, now)
				processed++
			default:
				i = limit
			}
		}
	}
	return processed
}

func (e *Engine) applyEvent(evt model.LogEvent, now time.Time) {
	e.lastID++
	evt.ID = e.lastID
	evt.ReceivedAt = now
	if evt.Level == model.LevelError {
		e.errorsPerMin.Add(1, now)
		e.errors++
		e.sourceErrors[evt.SourceKey]++
		e.ensureSourceErrorRate(evt.SourceKey).Add(1, now)
		if evt.ListenerID != "" {
			e.listenerErrors[evt.ListenerID]++
			e.ensureListenerErrorRate(evt.ListenerID).Add(1, now)
		}
	}

	source := e.sources[evt.SourceKey]
	if source == nil {
		if len(e.sources) >= e.limits.MaxSources {
			e.overflowSources++
			e.overflowBuffer.AppendWithLevel(evt.Line, uint8(evt.Level))
			overflow := e.ensureSource("overflow")
			e.appendLine(overflow.Key, evt.Line, evt.Level)
			return
		}
		source = e.ensureSource(evt.SourceKey)
	}
	source.LastSeen = now
	if evt.Level == model.LevelError {
		source.Errors++
	}
	e.appendLine(source.Key, evt.Line, evt.Level)
}

func (e *Engine) appendLine(sourceKey, line string, level model.Level) {
	buf := e.perSource[sourceKey]
	if buf == nil {
		buf = store.NewRingBuffer(e.limits.PerSourceBufferLines)
		e.perSource[sourceKey] = buf
	}
	buf.AppendWithLevel(line, uint8(level))
	e.globalBuf.AppendWithLevel(line, uint8(level))
}

func (e *Engine) ensureSource(key string) *model.Source {
	source := &model.Source{Key: key, Name: key}
	e.sources[key] = source
	return source
}
