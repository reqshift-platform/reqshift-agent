package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/health"
)

// Server exposes HTTP endpoints for health probes and metrics.
type Server struct {
	httpServer *http.Server
	monitor    *health.Monitor
	version    string
	startedAt  time.Time
}

// New creates a server bound to the given address.
func New(listen string, mon *health.Monitor, version string) *Server {
	s := &Server{
		monitor:   mon,
		version:   version,
		startedAt: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	s.httpServer = &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

// Mux returns the underlying ServeMux so callers can add routes (e.g. /metrics).
func (s *Server) Mux() *http.ServeMux {
	return s.httpServer.Handler.(*http.ServeMux)
}

// Start begins serving in a goroutine. Returns immediately.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()
	slog.Info("http server started", "addr", s.httpServer.Addr)
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

type healthzResponse struct {
	Status  string  `json:"status"`
	Uptime  float64 `json:"uptime"`
	Version string  `json:"version"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthzResponse{
		Status:  "ok",
		Uptime:  time.Since(s.startedAt).Seconds(),
		Version: s.version,
	})
}

type readyzResponse struct {
	Ready      bool              `json:"ready"`
	Connectors map[string]string `json:"connectors"`
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	snap := s.monitor.Snapshot()

	ready := false
	for _, status := range snap.ConnectorStatus {
		if status == "healthy" {
			ready = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if !ready && len(snap.ConnectorStatus) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(readyzResponse{
		Ready:      ready,
		Connectors: snap.ConnectorStatus,
	})
}
