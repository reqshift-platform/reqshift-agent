package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/internal/health"
	"github.com/reqshift-platform/reqshift-agent/internal/push"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

const heartbeatInterval = 30 * time.Second

// Scheduler manages periodic sync and heartbeat cycles.
//
// For each connector:
//   - Full sync at the configured interval (default 5min)
//   - Heartbeat every 30s
//
// All syncs are sequential per connector but concurrent across connectors.
type Scheduler struct {
	registry *connector.Registry
	pusher   *push.Client
	health   *health.Monitor
	cfg      *config.Config
	version  string
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func New(registry *connector.Registry, pusher *push.Client,
	healthMon *health.Monitor, cfg *config.Config, version string) *Scheduler {
	return &Scheduler{
		registry: registry,
		pusher:   pusher,
		health:   healthMon,
		cfg:      cfg,
		version:  version,
		stopCh:   make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	for _, entry := range s.registry.All() {
		s.wg.Add(1)
		go s.syncLoop(entry.Connector, entry.SyncInterval)
	}

	s.wg.Add(1)
	go s.heartbeatLoop()
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) syncLoop(conn connector.Connector, interval time.Duration) {
	defer s.wg.Done()

	s.doSync(conn)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.doSync(conn)
		case <-s.stopCh:
			return
		}
	}
}

const syncTimeout = 2 * time.Minute

func (s *Scheduler) doSync(conn connector.Connector) {
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	start := time.Now()
	logger := slog.With("connector", conn.Name(), "type", conn.Type())
	logger.Info("sync starting")

	payload := &models.SyncPayload{
		AgentID:       s.cfg.Agent.ID,
		AgentVersion:  s.version,
		Timestamp:     time.Now(),
		ConnectorType: conn.Type(),
		ConnectorName: conn.Name(),
	}

	specs, err := conn.FetchSpecs(ctx)
	if err != nil {
		logger.Error("fetch specs failed", "error", err)
		s.health.RecordError(conn.Name(), err)
	} else {
		payload.Specs = specs
	}

	metrics, err := conn.FetchMetrics(ctx)
	if err != nil {
		logger.Error("fetch metrics failed", "error", err)
	} else {
		payload.Metrics = metrics
	}

	payload.Health = s.health.Snapshot()

	result, err := s.pusher.PushSync(ctx, payload)
	if err != nil {
		logger.Error("push to cloud failed", "error", err)
		s.health.RecordError("cloud", err)
		return
	}

	s.health.RecordSuccess(conn.Name())
	logger.Info("sync complete",
		"specs", result.SpecsIngested,
		"metrics", result.MetricsIngested,
		"duration", time.Since(start))
}

func (s *Scheduler) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			snapshot := s.health.Snapshot()
			if err := s.pusher.PushHeartbeat(ctx, snapshot); err != nil {
				slog.Error("heartbeat failed", "error", err)
			}
			cancel()
		case <-s.stopCh:
			return
		}
	}
}
