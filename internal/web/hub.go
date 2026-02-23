package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"gelato/internal/app"
	"gelato/internal/engine"
)

type hubConfig struct {
	FlushInterval     time.Duration
	FullSnapshotEvery time.Duration
	MaxLogBatch       int
	WriteTimeout      time.Duration
}

type hub struct {
	rt  *app.Runtime
	cfg hubConfig

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	clients map[*wsClient]struct{}

	seq          uint64
	lastSnap     engine.Snapshot
	hasSnap      bool
	lastSnapSeq  uint64
	lastFullTime time.Time

	coalescedFrames uint64
	droppedClients  uint64
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte

	closeOnce sync.Once
	closed    chan struct{}
}

func newHub(parent context.Context, rt *app.Runtime, cfg hubConfig) *hub {
	ctx, cancel := context.WithCancel(parent)
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 100 * time.Millisecond
	}
	if cfg.FullSnapshotEvery <= 0 {
		cfg.FullSnapshotEvery = 5 * time.Second
	}
	if cfg.MaxLogBatch <= 0 {
		cfg.MaxLogBatch = 2000
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 2 * time.Second
	}
	return &hub{
		rt:      rt,
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		clients: make(map[*wsClient]struct{}),
	}
}

func (h *hub) run() {
	ticker := time.NewTicker(h.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			h.closeAll()
			return
		case <-ticker.C:
			h.broadcastTick()
		}
	}
}

func (h *hub) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	client := &wsClient{
		conn:   conn,
		send:   make(chan []byte, 1),
		closed: make(chan struct{}),
	}
	h.addClient(client)

	go h.writeLoop(client)

	hello, _ := json.Marshal(WSHelloFrame{
		Type:            "hello",
		Version:         "v1",
		ServerTime:      time.Now(),
		FlushMS:         int(h.cfg.FlushInterval / time.Millisecond),
		MaxLogBatch:     h.cfg.MaxLogBatch,
		FullSnapshotSec: int(h.cfg.FullSnapshotEvery / time.Second),
	})
	client.enqueue(hello, h)

	// Send an initial full snapshot immediately for fast first paint.
	snap := h.rt.Snapshot("active")
	seq := atomic.AddUint64(&h.seq, 1)
	h.mu.Lock()
	h.lastSnap = snap
	h.hasSnap = true
	h.lastSnapSeq = seq
	h.lastFullTime = time.Now()
	h.mu.Unlock()

	frame, _ := json.Marshal(WSSnapshotFrame{
		Type:     "snapshot",
		Seq:      seq,
		Reason:   "initial",
		Snapshot: snap,
	})
	client.enqueue(frame, h)
}

func (h *hub) writeLoop(c *wsClient) {
	defer h.removeClient(c)
	for {
		select {
		case <-h.ctx.Done():
			c.close()
			return
		case <-c.closed:
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(h.ctx, h.cfg.WriteTimeout)
			err := c.conn.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				atomic.AddUint64(&h.droppedClients, 1)
				c.close()
				return
			}
		}
	}
}

func (h *hub) addClient(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *hub) removeClient(c *wsClient) {
	c.close()
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *hub) closeAll() {
	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = map[*wsClient]struct{}{}
	h.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
}

func (h *hub) broadcastTick() {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	prev := h.lastSnap
	hasPrev := h.hasSnap
	prevSeq := h.lastSnapSeq
	lastFull := h.lastFullTime
	h.mu.Unlock()

	snap := h.rt.Snapshot("active")
	now := time.Now()

	if !hasPrev || now.Sub(lastFull) >= h.cfg.FullSnapshotEvery {
		seq := atomic.AddUint64(&h.seq, 1)
		frame, err := json.Marshal(WSSnapshotFrame{
			Type:     "snapshot",
			Seq:      seq,
			Reason:   "periodic",
			Snapshot: snap,
		})
		if err != nil {
			log.Printf("web hub: snapshot marshal failed: %v", err)
			return
		}
		h.mu.Lock()
		h.lastSnap = snap
		h.hasSnap = true
		h.lastSnapSeq = seq
		h.lastFullTime = now
		h.mu.Unlock()
		h.broadcast(frame)
		return
	}

	nextSeq := atomic.AddUint64(&h.seq, 1)
	delta := buildDelta(prev, snap, prevSeq, nextSeq, h.cfg.MaxLogBatch)
	if delta.needsSnapshot {
		frame, err := json.Marshal(WSSnapshotFrame{
			Type:     "snapshot",
			Seq:      nextSeq,
			Reason:   "resync",
			Snapshot: snap,
		})
		if err != nil {
			log.Printf("web hub: resync snapshot marshal failed: %v", err)
			return
		}
		h.mu.Lock()
		h.lastSnap = snap
		h.hasSnap = true
		h.lastSnapSeq = nextSeq
		h.lastFullTime = now
		h.mu.Unlock()
		h.broadcast(frame)
		return
	}
	if delta.frame.Type == "" {
		return
	}
	frame, err := json.Marshal(delta.frame)
	if err != nil {
		log.Printf("web hub: delta marshal failed: %v", err)
		return
	}
	h.mu.Lock()
	h.lastSnap = snap
	h.hasSnap = true
	h.lastSnapSeq = nextSeq
	h.mu.Unlock()
	h.broadcast(frame)
}

func (h *hub) broadcast(msg []byte) {
	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.enqueue(msg, h)
	}
}

func (c *wsClient) enqueue(msg []byte, h *hub) {
	select {
	case c.send <- msg:
	default:
		select {
		case <-c.send:
		default:
		}
		atomic.AddUint64(&h.coalescedFrames, 1)
		select {
		case c.send <- msg:
		default:
			c.close()
		}
	}
}

func (c *wsClient) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close(websocket.StatusNormalClosure, "bye")
	})
}
