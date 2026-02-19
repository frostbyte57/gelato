package engine

import (
	"context"
	"errors"
	"hash/fnv"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"gelato/internal/config"
	"gelato/internal/model"
	"gelato/internal/server"
	"gelato/internal/store"
)

type CommandType int

const (
	CommandAddListener CommandType = iota
	CommandRemoveListener
	CommandRetryListener
	CommandSetFilters
	CommandRenameSource
	CommandRecolorSource
	CommandToggleMuteSource
	CommandClearSource
)

type Command struct {
	Type       CommandType
	ListenerID string
	BindHost   string
	Port       int
	Filters    Filters
	SourceKey  string
	Value      string
	RespCh     chan CommandResult
}

type CommandResult struct {
	Listener model.Listener
	Err      error
}

type Filters struct {
	LevelMask  uint32
	SearchText string
	SourceKey  string
}

type SnapshotRequest struct {
	View string
}

type Snapshot struct {
	Listeners          []model.Listener
	ListenerStats      map[string]ListenerStats
	Sources            []model.Source
	SourceStats        map[string]SourceStats
	Lines              []string
	LineLevels         []model.Level
	Dropped            uint64
	Errors             uint64
	OverflowSources    uint64
	ShardDrops         []uint64
	SourceErrorRates   map[string][]uint64
	SourceDropRates    map[string][]uint64
	ListenerErrorRates map[string][]uint64
	ListenerDropRates  map[string][]uint64
	Stats              StatsSnapshot
}

type SourceStats struct {
	ActiveConns int
	Dropped     uint64
	Errors      uint64
}

type ListenerStats struct {
	ActiveConns int
	Dropped     uint64
	Rejected    uint64
	Errors      uint64
}

type StatsSnapshot struct {
	LogsPerSec   []uint64
	ErrorsPerMin []uint64
	DropsPerMin  []uint64
}

type Engine struct {
	limits config.Limits

	uiCmdCh    chan Command
	snapReqCh  chan SnapshotRequest
	snapRespCh chan Snapshot
	eventCh    []chan model.LogEvent

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
	}

	for i := range engine.eventCh {
		engine.eventCh[i] = make(chan model.LogEvent, limits.QueueSizePerShard)
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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
			e.drainEvents(256)
		}
	}
}

func (e *Engine) AssignShard(sourceKey string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sourceKey))
	return int(h.Sum32()) % len(e.eventCh)
}

func (e *Engine) handleCommand(cmd Command) error {
	switch cmd.Type {
	case CommandAddListener:
		return e.addListener(cmd)
	case CommandRemoveListener:
		return e.removeListener(cmd)
	case CommandRetryListener:
		return e.retryListener(cmd)
	case CommandSetFilters:
		e.filters = cmd.Filters
		return nil
	case CommandRenameSource:
		return e.renameSource(cmd)
	case CommandRecolorSource:
		return e.recolorSource(cmd)
	case CommandToggleMuteSource:
		return e.toggleMuteSource(cmd)
	case CommandClearSource:
		return e.clearSource(cmd)
	default:
		return errors.New("unknown command")
	}
}

func (e *Engine) drainEvents(batchSize int) {
	now := time.Now()
	processed := uint64(0)
	for _, ch := range e.eventCh {
		for i := 0; i < batchSize; i++ {
			select {
			case evt := <-ch:
				e.applyEvent(evt, now)
				processed++
			default:
				i = batchSize
			}
		}
	}
	if processed > 0 {
		e.logsPerSec.Add(processed, now)
	}

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

func (e *Engine) renameSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Name = cmd.Value
	return nil
}

func (e *Engine) recolorSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Color = cmd.Value
	return nil
}

func (e *Engine) toggleMuteSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Muted = !source.Muted
	return nil
}

func (e *Engine) clearSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	buf := e.perSource[cmd.SourceKey]
	if buf != nil {
		buf.Clear()
	}
	return nil
}

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
	lines, levelsRaw := e.collectLines()
	levels := convertLevels(levelsRaw)
	cache := &e.filterCache
	version := e.currentBufferVersion()
	if cache.version == version && cache.sourceKey == e.filters.SourceKey && cache.levelMask == e.filters.LevelMask && cache.search == e.filters.SearchText {
		return cache.lines, cache.levels
	}
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

func (e *Engine) addListener(cmd Command) error {
	if e.startListenerFn == nil {
		return errors.New("listener handler not attached")
	}
	id := cmd.ListenerID
	if id == "" {
		id = fmtListenerID(cmd.BindHost, cmd.Port)
	}
	listener := model.Listener{
		ID:        id,
		BindHost:  cmd.BindHost,
		Port:      cmd.Port,
		Status:    model.ListenerStarting,
		StartedAt: time.Now(),
	}
	e.listeners[id] = listener

	addr, err := e.startListenerFn(context.Background(), listener)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[id] = listener
		sendResult(cmd, listener, err)
		return err
	}

	host, port := splitAddress(addr)
	listener.BindHost = host
	listener.Port = port
	listener.Status = model.ListenerListening
	listener.ErrMsg = ""
	e.listeners[id] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func (e *Engine) removeListener(cmd Command) error {
	listener, ok := e.listeners[cmd.ListenerID]
	if !ok {
		return errors.New("listener not found")
	}
	if e.stopListenerFn == nil {
		return errors.New("listener handler not attached")
	}

	err := e.stopListenerFn(context.Background(), cmd.ListenerID)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[cmd.ListenerID] = listener
		sendResult(cmd, listener, err)
		return err
	}
	listener.Status = model.ListenerStopped
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func (e *Engine) retryListener(cmd Command) error {
	listener, ok := e.listeners[cmd.ListenerID]
	if !ok {
		return errors.New("listener not found")
	}
	if e.startListenerFn == nil {
		return errors.New("listener handler not attached")
	}

	listener.Status = model.ListenerStarting
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener

	addr, err := e.startListenerFn(context.Background(), listener)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[cmd.ListenerID] = listener
		sendResult(cmd, listener, err)
		return err
	}

	host, port := splitAddress(addr)
	listener.BindHost = host
	listener.Port = port
	listener.Status = model.ListenerListening
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func splitAddress(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0
	}
	return host, port
}

func fmtListenerID(host string, port int) string {
	if port == 0 {
		return host
	}
	return host + ":" + strconv.Itoa(port)
}

func sendResult(cmd Command, listener model.Listener, err error) {
	if cmd.RespCh == nil {
		return
	}
	cmd.RespCh <- CommandResult{Listener: listener, Err: err}
}

type statsAggregator struct {
	queue []server.EngineStatsUpdate
	tick  *time.Ticker
}

type filterCache struct {
	version   uint64
	sourceKey string
	levelMask uint32
	search    string
	lines     []string
	levels    []model.Level
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

func filterLines(lines []string, levels []model.Level, filter string) ([]string, []model.Level) {
	if filter == "" {
		return lines, levels
	}
	filtered := make([]string, 0, len(lines))
	filteredLevels := make([]model.Level, 0, len(lines))
	needle := strings.ToLower(filter)
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			filtered = append(filtered, line)
			filteredLevels = append(filteredLevels, levels[i])
		}
	}
	return filtered, filteredLevels
}

func filterLinesByLevel(lines []string, levels []model.Level, levelMask uint32) ([]string, []model.Level) {
	if levelMask == model.LevelMaskAll {
		return lines, levels
	}
	filtered := make([]string, 0, len(lines))
	filteredLevels := make([]model.Level, 0, len(lines))
	for i := range lines {
		level := levels[i]
		if levelMask&levelToMask(level) != 0 {
			filtered = append(filtered, lines[i])
			filteredLevels = append(filteredLevels, level)
		}
	}
	return filtered, filteredLevels
}

func levelToMask(level model.Level) uint32 {
	switch level {
	case model.LevelDebug:
		return model.LevelMaskDebug
	case model.LevelInfo:
		return model.LevelMaskInfo
	case model.LevelWarn:
		return model.LevelMaskWarn
	case model.LevelError:
		return model.LevelMaskError
	default:
		return model.LevelMaskAll
	}
}

func convertLevels(levels []uint8) []model.Level {
	out := make([]model.Level, len(levels))
	for i, level := range levels {
		out[i] = model.Level(level)
	}
	return out
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
