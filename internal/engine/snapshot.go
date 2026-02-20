package engine

import (
	"sort"
	"time"

	"gelato/internal/model"
)

func (e *Engine) buildSnapshot(req SnapshotRequest) Snapshot {
	listeners := e.collectListeners()
	lines, levels := e.collectFilteredLines()
	listenerStats := map[string]ListenerStats{}
	if e.statsFn != nil {
		listenerStats = e.statsFn()
	}
	sourceStats := map[string]SourceStats{}
	if e.sourceStatsFn != nil {
		sourceStats = e.sourceStatsFn()
	}
	listenerStats = mergeListenerStats(listenerStats, e.listenerDrops, e.listenerErrors)
	sourceStats = mergeSourceStats(sourceStats, e.sourceDrops, e.sourceErrors)
	sources := buildSources(e.sources, sourceStats)
	now := time.Now()
	return Snapshot{
		Listeners:          listeners,
		ListenerStats:      listenerStats,
		Sources:            sources,
		SourceStats:        sourceStats,
		Lines:              lines,
		LineLevels:         levels,
		Dropped:            e.dropped,
		Errors:             e.errors,
		OverflowSources:    e.overflowSources,
		SourceErrorRates:   e.buildSourceErrorRates(now),
		ListenerErrorRates: e.buildListenerErrorRates(now),
		ListenerDropRates:  e.buildListenerDropRates(now),
		SourceDropRates:    e.buildSourceDropRates(now),
		ShardDrops:         append([]uint64(nil), e.shardDrops...),
		Stats: StatsSnapshot{
			LogsPerSec:   e.logsPerSec.Snapshot(now),
			ErrorsPerMin: e.errorsPerMin.Snapshot(now),
			DropsPerMin:  e.dropsPerMin.Snapshot(now),
		},
	}
}

func (e *Engine) collectFilteredLines() ([]string, []model.Level) {
	cache := &e.filterCache
	version := e.currentBufferVersion()
	if cache.version == version && cache.sourceKey == e.filters.SourceKey && cache.levelMask == e.filters.LevelMask && cache.search == e.filters.SearchText {
		return cache.lines, cache.levels
	}
	lines, levelsRaw := e.collectLines()
	levels := convertLevels(levelsRaw)
	if e.filters.SearchText != "" {
		lines, levels = filterLines(lines, levels, e.filters.SearchText)
	}
	if e.filters.LevelMask != model.LevelMaskAll {
		lines, levels = filterLinesByLevel(lines, levels, e.filters.LevelMask)
	}
	cache.version = version
	cache.sourceKey = e.filters.SourceKey
	cache.levelMask = e.filters.LevelMask
	cache.search = e.filters.SearchText
	cache.lines = lines
	cache.levels = levels
	return lines, levels
}

func (e *Engine) currentBufferVersion() uint64 {
	if e.filters.SourceKey != "" {
		if buf, ok := e.perSource[e.filters.SourceKey]; ok {
			return buf.Version()
		}
		return 0
	}
	return e.globalBuf.Version()
}

func (e *Engine) collectListeners() []model.Listener {
	listeners := make([]model.Listener, 0, len(e.listeners))
	for _, listener := range e.listeners {
		listeners = append(listeners, listener)
	}
	return listeners
}

func (e *Engine) collectLines() ([]string, []uint8) {
	if e.filters.SourceKey != "" {
		if buf, ok := e.perSource[e.filters.SourceKey]; ok {
			return buf.ItemsWithLevels()
		}
		return nil, nil
	}
	return e.globalBuf.ItemsWithLevels()
}

func buildSources(sources map[string]*model.Source, stats map[string]SourceStats) []model.Source {
	out := make([]model.Source, 0, len(sources))
	for _, source := range sources {
		copy := *source
		if stat, ok := stats[source.Key]; ok {
			copy.ActiveConns = stat.ActiveConns
			copy.Dropped = stat.Dropped
			copy.Errors = stat.Errors
		}
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActiveConns == out[j].ActiveConns {
			return out[i].Dropped > out[j].Dropped
		}
		return out[i].ActiveConns > out[j].ActiveConns
	})
	return out
}

func mergeListenerStats(base map[string]ListenerStats, drops map[string]uint64, errors map[string]uint64) map[string]ListenerStats {
	out := make(map[string]ListenerStats, len(base)+len(drops)+len(errors))
	for id, stat := range base {
		stat.Dropped += drops[id]
		stat.Errors += errors[id]
		out[id] = stat
	}
	for id, drop := range drops {
		if _, ok := out[id]; ok {
			continue
		}
		out[id] = ListenerStats{Dropped: drop, Errors: errors[id]}
	}
	for id, errCount := range errors {
		stat := out[id]
		stat.Errors += errCount
		out[id] = stat
	}
	return out
}

func mergeSourceStats(base map[string]SourceStats, drops map[string]uint64, errors map[string]uint64) map[string]SourceStats {
	out := make(map[string]SourceStats, len(base)+len(drops)+len(errors))
	for key, stat := range base {
		stat.Dropped += drops[key]
		stat.Errors += errors[key]
		out[key] = stat
	}
	for key, drop := range drops {
		if _, ok := out[key]; ok {
			continue
		}
		out[key] = SourceStats{Dropped: drop, Errors: errors[key]}
	}
	for key, errCount := range errors {
		stat := out[key]
		stat.Errors += errCount
		out[key] = stat
	}
	return out
}
