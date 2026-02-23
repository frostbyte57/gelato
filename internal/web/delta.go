package web

import (
	"reflect"

	"gelato/internal/engine"
	"gelato/internal/model"
)

type deltaResult struct {
	frame         WSDeltaFrame
	needsSnapshot bool
	reason        string
}

func buildDelta(prev engine.Snapshot, curr engine.Snapshot, fromSeq, toSeq uint64, maxLogBatch int) deltaResult {
	if maxLogBatch <= 0 {
		maxLogBatch = 2000
	}

	// If filters changed (or any non-append line mutation), resync with a full snapshot.
	newLogs, truncateHint, ok := appendOnlyLogs(prev, curr, maxLogBatch)
	if !ok {
		return deltaResult{needsSnapshot: true, reason: "resync"}
	}

	listenerPatches := diffListeners(prev, curr)
	sourcePatches := diffSources(prev, curr)
	statsChanged := !reflect.DeepEqual(prev.Stats, curr.Stats) ||
		prev.Dropped != curr.Dropped ||
		prev.Errors != curr.Errors ||
		prev.OverflowSources != curr.OverflowSources ||
		!reflect.DeepEqual(prev.ShardDrops, curr.ShardDrops) ||
		!reflect.DeepEqual(prev.ListenerErrorRates, curr.ListenerErrorRates) ||
		!reflect.DeepEqual(prev.ListenerDropRates, curr.ListenerDropRates) ||
		!reflect.DeepEqual(prev.SourceErrorRates, curr.SourceErrorRates) ||
		!reflect.DeepEqual(prev.SourceDropRates, curr.SourceDropRates)

	if len(newLogs) == 0 && len(listenerPatches) == 0 && len(sourcePatches) == 0 && !statsChanged {
		return deltaResult{}
	}

	return deltaResult{
		frame: WSDeltaFrame{
			Type:            "delta",
			FromSeq:         fromSeq,
			ToSeq:           toSeq,
			NewLogs:         newLogs,
			ListenerPatches: listenerPatches,
			SourcePatches:   sourcePatches,
			Stats:           snapshotStats(curr),
			TruncateHint:    truncateHint,
		},
	}
}

func appendOnlyLogs(prev, curr engine.Snapshot, maxLogBatch int) ([]LogLineDTO, *int, bool) {
	if len(curr.Lines) < len(prev.Lines) {
		return nil, nil, false
	}
	for i := range prev.Lines {
		if prev.Lines[i] != curr.Lines[i] {
			return nil, nil, false
		}
	}

	start := len(prev.Lines)
	if start >= len(curr.Lines) {
		return nil, nil, true
	}

	totalNew := len(curr.Lines) - start
	if totalNew > maxLogBatch {
		start = len(curr.Lines) - maxLogBatch
	}
	out := make([]LogLineDTO, 0, len(curr.Lines)-start)
	for i := start; i < len(curr.Lines); i++ {
		level := model.LevelUnknown
		if i < len(curr.LineLevels) {
			level = curr.LineLevels[i]
		}
		out = append(out, LogLineDTO{Line: curr.Lines[i], Level: level})
	}

	var truncateHint *int
	if totalNew > maxLogBatch {
		n := totalNew - maxLogBatch
		truncateHint = &n
	}
	return out, truncateHint, true
}

func diffListeners(prev, curr engine.Snapshot) []ListenerPatchDTO {
	prevListeners := make(map[string]model.Listener, len(prev.Listeners))
	for _, l := range prev.Listeners {
		prevListeners[l.ID] = l
	}
	prevStats := prev.ListenerStats

	out := []ListenerPatchDTO{}
	for _, l := range curr.Listeners {
		pl, ok := prevListeners[l.ID]
		if ok && reflect.DeepEqual(pl, l) && reflect.DeepEqual(prevStats[l.ID], curr.ListenerStats[l.ID]) {
			continue
		}
		out = append(out, ListenerPatchDTO{
			Listener: l,
			Stats:    curr.ListenerStats[l.ID],
		})
	}
	return out
}

func diffSources(prev, curr engine.Snapshot) []SourcePatchDTO {
	prevSources := make(map[string]model.Source, len(prev.Sources))
	for _, s := range prev.Sources {
		prevSources[s.Key] = s
	}
	prevStats := prev.SourceStats

	out := []SourcePatchDTO{}
	for _, s := range curr.Sources {
		ps, ok := prevSources[s.Key]
		if ok && reflect.DeepEqual(ps, s) && reflect.DeepEqual(prevStats[s.Key], curr.SourceStats[s.Key]) {
			continue
		}
		out = append(out, SourcePatchDTO{
			Source: s,
			Stats:  curr.SourceStats[s.Key],
		})
	}
	return out
}

func snapshotStats(s engine.Snapshot) DeltaStatsDTO {
	return DeltaStatsDTO{
		Stats:              s.Stats,
		Dropped:            s.Dropped,
		Errors:             s.Errors,
		OverflowSources:    s.OverflowSources,
		ShardDrops:         append([]uint64(nil), s.ShardDrops...),
		ListenerErrorRates: cloneRates(s.ListenerErrorRates),
		ListenerDropRates:  cloneRates(s.ListenerDropRates),
		SourceErrorRates:   cloneRates(s.SourceErrorRates),
		SourceDropRates:    cloneRates(s.SourceDropRates),
	}
}

func cloneRates(in map[string][]uint64) map[string][]uint64 {
	out := make(map[string][]uint64, len(in))
	for k, v := range in {
		out[k] = append([]uint64(nil), v...)
	}
	return out
}
