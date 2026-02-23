package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"gelato/internal/app"
	"gelato/internal/engine"
	"gelato/internal/model"
)

//go:embed static/*
var embeddedStatic embed.FS

type Options struct {
	Host             string
	Port             int
	AssetsDir        string
	WSFlushMS        int
	WSMaxLogBatch    int
	WSSnapshotSecs   int
	WSWriteTimeoutMS int
}

type Server struct {
	rt   *app.Runtime
	opts Options

	httpServer *http.Server
	hub        *hub

	filtersMu sync.RWMutex
	filters   engine.Filters
}

func New(rt *app.Runtime, opts Options) *Server {
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = 8080
	}
	if opts.WSFlushMS <= 0 {
		opts.WSFlushMS = 100
	}
	if opts.WSMaxLogBatch <= 0 {
		opts.WSMaxLogBatch = 2000
	}
	if opts.WSSnapshotSecs <= 0 {
		opts.WSSnapshotSecs = 5
	}
	if opts.WSWriteTimeoutMS <= 0 {
		opts.WSWriteTimeoutMS = 2000
	}

	s := &Server{
		rt:   rt,
		opts: opts,
		filters: engine.Filters{
			LevelMask: model.LevelMaskAll,
		},
	}
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:    net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port)),
		Handler: mux,
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	s.hub = newHub(ctx, s.rt, hubConfig{
		FlushInterval:     time.Duration(s.opts.WSFlushMS) * time.Millisecond,
		FullSnapshotEvery: time.Duration(s.opts.WSSnapshotSecs) * time.Second,
		MaxLogBatch:       s.opts.WSMaxLogBatch,
		WriteTimeout:      time.Duration(s.opts.WSWriteTimeoutMS) * time.Millisecond,
	})
	go s.hub.run()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Addr() string {
	return "http://" + s.httpServer.Addr
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ws", s.handleWS)
	mux.HandleFunc("/api/v1/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/v1/filters", s.handleFilters)
	mux.HandleFunc("/api/v1/listeners", s.handleListeners)
	mux.HandleFunc("/api/v1/listeners/", s.handleListenerByID)
	mux.HandleFunc("/api/v1/sources/", s.handleSourceActions)
	mux.Handle("/", s.staticHandler())
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "hub_unavailable", "websocket hub not ready")
		return
	}
	s.hub.serveWS(w, r)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	snap := s.rt.Snapshot(r.URL.Query().Get("view"))
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleFilters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.filtersMu.RLock()
		f := s.filters
		s.filtersMu.RUnlock()
		writeJSON(w, http.StatusOK, FiltersDTO{
			LevelMask:  f.LevelMask,
			SearchText: f.SearchText,
			SourceKey:  f.SourceKey,
		})
	case http.MethodPatch:
		var body FiltersDTO
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "invalid json body")
			return
		}
		f := engine.Filters{
			LevelMask:  body.LevelMask,
			SearchText: strings.TrimSpace(body.SearchText),
			SourceKey:  strings.TrimSpace(body.SourceKey),
		}
		if f.LevelMask == 0 {
			f.LevelMask = model.LevelMaskAll
		}
		if _, err := s.sendCommand(r.Context(), engine.Command{Type: engine.CommandSetFilters, Filters: f}); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "command_failed", err.Error())
			return
		}
		s.filtersMu.Lock()
		s.filters = f
		s.filtersMu.Unlock()
		writeJSON(w, http.StatusOK, FiltersDTO{LevelMask: f.LevelMask, SearchText: f.SearchText, SourceKey: f.SourceKey})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *Server) handleListeners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snap := s.rt.Snapshot("active")
		out := struct {
			Listeners     []model.Listener                `json:"listeners"`
			ListenerStats map[string]engine.ListenerStats `json:"listenerStats"`
		}{
			Listeners:     snap.Listeners,
			ListenerStats: snap.ListenerStats,
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req CreateListenerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "invalid json body")
			return
		}
		req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
		if req.Protocol == "" {
			req.Protocol = "tcp"
		}
		if req.Protocol != "tcp" {
			writeJSONError(w, http.StatusBadRequest, "unsupported_protocol", "only tcp is supported")
			return
		}
		host := strings.TrimSpace(req.BindHost)
		if host == "" {
			host = s.rt.Limits.DefaultBindHost
		}
		if err := validateHost(host); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_host", err.Error())
			return
		}
		if req.Port < 1 || req.Port > 65535 {
			writeJSONError(w, http.StatusBadRequest, "invalid_port", "port must be between 1 and 65535")
			return
		}
		res, err := s.sendCommand(r.Context(), engine.Command{
			Type:     engine.CommandAddListener,
			BindHost: host,
			Port:     req.Port,
		})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "command_failed", err.Error())
			return
		}
		dto := CommandResultDTO{OK: res.Err == nil}
		if res.Listener.ID != "" {
			l := res.Listener
			dto.Listener = &l
		}
		if res.Err != nil {
			dto.Error = &APIError{Code: "listener_error", Message: res.Err.Error()}
		}
		status := http.StatusCreated
		if res.Err != nil {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, dto)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *Server) handleListenerByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/listeners/")
	if rest == "" {
		writeJSONError(w, http.StatusNotFound, "not_found", "listener not found")
		return
	}
	if strings.HasSuffix(rest, "/retry") {
		id := strings.TrimSuffix(rest, "/retry")
		id = strings.TrimSuffix(id, "/")
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		s.handleListenerCommand(w, r, id, engine.CommandRetryListener)
		return
	}
	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	id := strings.TrimSuffix(rest, "/")
	s.handleListenerCommand(w, r, id, engine.CommandRemoveListener)
}

func (s *Server) handleListenerCommand(w http.ResponseWriter, r *http.Request, id string, typ engine.CommandType) {
	id = strings.TrimSpace(id)
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_listener_id", "listener id required")
		return
	}
	res, err := s.sendCommand(r.Context(), engine.Command{Type: typ, ListenerID: id})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "command_failed", err.Error())
		return
	}
	dto := CommandResultDTO{OK: res.Err == nil}
	if res.Listener.ID != "" {
		l := res.Listener
		dto.Listener = &l
	}
	if res.Err != nil {
		dto.Error = &APIError{Code: "listener_error", Message: res.Err.Error()}
	}
	status := http.StatusOK
	if res.Err != nil {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, dto)
}

func (s *Server) handleSourceActions(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/sources/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeJSONError(w, http.StatusNotFound, "not_found", "source not found")
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 {
		writeJSONError(w, http.StatusNotFound, "not_found", "source not found")
		return
	}
	key, err := urlPathUnescape(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_source_key", "invalid source key")
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "toggle-mute":
			_, err = s.sendCommand(r.Context(), engine.Command{Type: engine.CommandToggleMuteSource, SourceKey: key})
		case "clear":
			_, err = s.sendCommand(r.Context(), engine.Command{Type: engine.CommandClearSource, SourceKey: key})
		default:
			writeJSONError(w, http.StatusNotFound, "not_found", "action not found")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "command_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if len(parts) == 1 && r.Method == http.MethodPatch {
		var req UpdateSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_json", "invalid json body")
			return
		}
		if req.Name != nil {
			if _, err := s.sendCommand(r.Context(), engine.Command{Type: engine.CommandRenameSource, SourceKey: key, Value: strings.TrimSpace(*req.Name)}); err != nil {
				writeJSONError(w, http.StatusBadRequest, "command_failed", err.Error())
				return
			}
		}
		if req.Color != nil {
			if _, err := s.sendCommand(r.Context(), engine.Command{Type: engine.CommandRecolorSource, SourceKey: key, Value: strings.TrimSpace(*req.Color)}); err != nil {
				writeJSONError(w, http.StatusBadRequest, "command_failed", err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func (s *Server) sendCommand(ctx context.Context, cmd engine.Command) (engine.CommandResult, error) {
	waitResp := commandExpectsResponse(cmd.Type)
	var respCh chan engine.CommandResult
	if waitResp {
		respCh = make(chan engine.CommandResult, 1)
		cmd.RespCh = respCh
	}
	select {
	case s.rt.Engine.UICmdCh() <- cmd:
	case <-ctx.Done():
		return engine.CommandResult{}, ctx.Err()
	}
	if !waitResp {
		return engine.CommandResult{}, nil
	}

	select {
	case res := <-respCh:
		if res.Err != nil {
			return res, res.Err
		}
		return res, nil
	case <-ctx.Done():
		return engine.CommandResult{}, ctx.Err()
	case <-time.After(5 * time.Second):
		return engine.CommandResult{}, errors.New("command timeout")
	}
}

func commandExpectsResponse(typ engine.CommandType) bool {
	switch typ {
	case engine.CommandAddListener, engine.CommandRemoveListener, engine.CommandRetryListener:
		return true
	default:
		return false
	}
}

func (s *Server) staticHandler() http.Handler {
	fileServer := s.fileServer()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) fileServer() http.Handler {
	if s.opts.AssetsDir != "" {
		if st, err := os.Stat(s.opts.AssetsDir); err == nil && st.IsDir() {
			return spaFileServer(os.DirFS(s.opts.AssetsDir))
		}
	}
	if st, err := os.Stat("web/dist"); err == nil && st.IsDir() {
		return spaFileServer(os.DirFS("web/dist"))
	}
	sub, _ := fs.Sub(embeddedStatic, "static")
	return spaFileServer(sub)
}

func spaFileServer(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || clean == "" {
			serveIndexFile(root, w)
			return
		}
		if existsFS(root, clean) {
			files.ServeHTTP(w, r)
			return
		}
		serveIndexFile(root, w)
	})
}

func existsFS(root fs.FS, name string) bool {
	_, err := fs.Stat(root, name)
	return err == nil
}

func serveIndexFile(root fs.FS, w http.ResponseWriter) {
	body, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.Error(w, "frontend assets not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func validateHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return errors.New("host contains whitespace")
	}
	// Allow hostnames and IPs. Reject obviously broken host:port input at this layer.
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		if net.ParseIP(host) == nil {
			return errors.New("bindHost must be a host or IP without port")
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]APIError{
		"error": {Code: code, Message: msg},
	})
}

func urlPathUnescape(s string) (string, error) {
	return url.PathUnescape(s)
}
