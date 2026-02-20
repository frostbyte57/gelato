package server

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gelato/internal/config"
	"gelato/internal/model"
)

type ListenerInfo struct {
	ID      string
	Address string
}

type Server struct {
	limits config.Limits

	eventCh   []chan<- model.LogEvent
	statsSink func(EngineStatsUpdate)

	mu         sync.Mutex
	listeners  map[string]*listenerState
	totalConns int64
	sources    sync.Map
}

type listenerState struct {
	id          string
	listener    net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	activeConns int64
	dropped     uint64
	rejected    uint64
}

type sourceState struct {
	activeConns int64
	dropped     uint64
}

type EngineStatsUpdate struct {
	Dropped    uint64
	Errors     uint64
	ListenerID string
	SourceKey  string
	Shard      int
}

func New(limits config.Limits, eventCh []chan<- model.LogEvent) *Server {
	return &Server{
		limits:    limits,
		eventCh:   eventCh,
		listeners: make(map[string]*listenerState),
	}
}

func (s *Server) SetStatsSink(sink func(EngineStatsUpdate)) {
	s.statsSink = sink
}

func (s *Server) Start(ctx context.Context, l model.Listener) (ListenerInfo, error) {
	addr := net.JoinHostPort(l.BindHost, formatPort(l.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return ListenerInfo{}, err
	}

	listenerCtx, cancel := context.WithCancel(ctx)
	state := &listenerState{listener: ln, ctx: listenerCtx, cancel: cancel, id: l.ID}

	s.mu.Lock()
	s.listeners[l.ID] = state
	s.mu.Unlock()

	go s.acceptLoop(state)
	return ListenerInfo{ID: l.ID, Address: ln.Addr().String()}, nil
}

func (s *Server) Stop(id string) error {
	s.mu.Lock()
	state := s.listeners[id]
	delete(s.listeners, id)
	s.mu.Unlock()
	if state == nil {
		return errors.New("listener not found")
	}
	state.cancel()
	return state.listener.Close()
}

func (s *Server) acceptLoop(state *listenerState) {
	for {
		conn, err := state.listener.Accept()
		if err != nil {
			select {
			case <-state.ctx.Done():
				return
			default:
				continue
			}
		}
		if atomic.LoadInt64(&s.totalConns) >= int64(s.limits.MaxConnsGlobal) {
			atomic.AddUint64(&state.rejected, 1)
			s.emitDrop(EngineStatsUpdate{Errors: 1, ListenerID: state.id})
			_ = conn.Close()
			continue
		}
		if atomic.LoadInt64(&state.activeConns) >= int64(s.limits.MaxConnsPerListener) {
			atomic.AddUint64(&state.rejected, 1)
			s.emitDrop(EngineStatsUpdate{Errors: 1, ListenerID: state.id})
			_ = conn.Close()
			continue
		}
		atomic.AddInt64(&s.totalConns, 1)
		atomic.AddInt64(&state.activeConns, 1)
		go s.readLoop(state, conn)
	}
}

func (s *Server) readLoop(state *listenerState, conn net.Conn) {
	sourceKey := sourceKey(conn.RemoteAddr())
	source := s.getOrCreateSource(sourceKey)
	atomic.AddInt64(&source.activeConns, 1)

	defer func() {
		_ = conn.Close()
		atomic.AddInt64(&s.totalConns, -1)
		atomic.AddInt64(&state.activeConns, -1)
		atomic.AddInt64(&source.activeConns, -1)
	}()

	reader := bufio.NewReader(conn)

	for {
		select {
		case <-state.ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if len(line) == 0 {
					s.emitDrop(EngineStatsUpdate{Errors: 1, ListenerID: state.id, SourceKey: sourceKey})
					return
				}
			}
			line = strings.TrimRight(line, "\r\n")
			if len(line) > s.limits.MaxLineBytes {
				line = line[:s.limits.MaxLineBytes]
			}
			level := DetectLevel(line)
			evt := model.LogEvent{SourceKey: sourceKey, RemoteAddr: conn.RemoteAddr().String(), Line: line, ListenerID: state.id, Level: level}
			shard := s.assignShard(sourceKey)
			select {
			case s.eventCh[shard] <- evt:
			default:
				atomic.AddUint64(&state.dropped, 1)
				atomic.AddUint64(&source.dropped, 1)
				s.emitDrop(EngineStatsUpdate{Dropped: 1, ListenerID: state.id, SourceKey: sourceKey, Shard: shard})
			}
			if err != nil {
				return
			}
		}
	}
}

// RunReadLoopForTest executes readLoop with the provided connection and returns the drop count.
func (s *Server) RunReadLoopForTest(ctx context.Context, conn net.Conn, listenerID string) (uint64, error) {
	stateCtx, cancel := context.WithCancel(ctx)
	state := &listenerState{id: listenerID, ctx: stateCtx, cancel: cancel}
	atomic.AddInt64(&s.totalConns, 1)
	atomic.AddInt64(&state.activeConns, 1)

	done := make(chan struct{})
	go func() {
		s.readLoop(state, conn)
		close(done)
	}()

	select {
	case <-done:
		return atomic.LoadUint64(&state.dropped), nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *Server) assignShard(sourceKey string) int {
	if len(s.eventCh) == 0 {
		return 0
	}
	h := fnv32a(sourceKey)
	return int(h % uint32(len(s.eventCh)))
}

func (s *Server) ListenerStats() map[string]ListenerStats {
	stats := make(map[string]ListenerStats)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, state := range s.listeners {
		stats[id] = ListenerStats{
			ActiveConns: int(atomic.LoadInt64(&state.activeConns)),
			Dropped:     atomic.LoadUint64(&state.dropped),
			Rejected:    atomic.LoadUint64(&state.rejected),
		}
	}
	return stats
}

func (s *Server) SourceStats() map[string]SourceStats {
	stats := make(map[string]SourceStats)
	s.sources.Range(func(key, value any) bool {
		keyStr, ok := key.(string)
		if !ok {
			return true
		}
		source, ok := value.(*sourceState)
		if !ok {
			return true
		}
		stats[keyStr] = SourceStats{
			ActiveConns: int(atomic.LoadInt64(&source.activeConns)),
			Dropped:     atomic.LoadUint64(&source.dropped),
		}
		return true
	})
	return stats
}

func (s *Server) emitDrop(update EngineStatsUpdate) {
	if s.statsSink == nil {
		return
	}
	if update.Dropped == 0 && update.Errors == 0 {
		return
	}
	s.statsSink(update)
}

func (s *Server) getOrCreateSource(key string) *sourceState {
	if value, ok := s.sources.Load(key); ok {
		source, ok := value.(*sourceState)
		if ok {
			return source
		}
	}
	source := &sourceState{}
	actual, _ := s.sources.LoadOrStore(key, source)
	if stored, ok := actual.(*sourceState); ok {
		return stored
	}
	return source
}

type ListenerStats struct {
	ActiveConns int
	Dropped     uint64
	Rejected    uint64
}

type SourceStats struct {
	ActiveConns int
	Dropped     uint64
	Errors      uint64
}

func sourceKey(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	parts := strings.Split(addr.String(), ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return addr.String()
}

func formatPort(port int) string {
	return strconv.Itoa(port)
}

func fnv32a(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	var hash uint32 = offset32
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= prime32
	}
	return hash
}

func DetectLevel(line string) model.Level {
	upper := strings.ToUpper(line)
	if strings.Contains(upper, "ERROR") || strings.Contains(upper, " ERR ") {
		return model.LevelError
	}
	if strings.Contains(upper, "WARN") {
		return model.LevelWarn
	}
	if strings.Contains(upper, "INFO") {
		return model.LevelInfo
	}
	if strings.Contains(upper, "DEBUG") {
		return model.LevelDebug
	}
	return model.LevelUnknown
}
