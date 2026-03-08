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
	cancel   context.CancelFunc
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
	}
}

func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	for _, entry := range s.registry.All() {
		s.wg.Add(1)
		go s.syncLoop(ctx, entry.Connector, entry.SyncInterval)
	}

	s.wg.Add(1)
	go s.heartbeatLoop(ctx)
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *Scheduler) syncLoop(ctx context.Context, conn connector.Connector, interval time.Duration) {
	defer s.wg.Done()

	s.doSync(ctx, conn)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.doSync(ctx, conn)
		case <-ctx.Done():
			return
		}
	}
}

const syncTimeout = 2 * time.Minute

func (s *Scheduler) doSync(parentCtx context.Context, conn connector.Connector) {
	ctx, cancel := context.WithTimeout(parentCtx, syncTimeout)
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

	// Fetch specs and metrics concurrently.
	var (
		specs      []models.APISpec
		specsErr   error
		metrics    []models.APIMetrics
		metricsErr error
		wg         sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		specs, specsErr = conn.FetchSpecs(ctx)
	}()
	go func() {
		defer wg.Done()
		metrics, metricsErr = conn.FetchMetrics(ctx)
	}()
	wg.Wait()

	if specsErr != nil {
		logger.Error("fetch specs failed", "error", specsErr)
		s.health.RecordError(conn.Name(), specsErr)
	} else {
		payload.Specs = specs
	}

	if metricsErr != nil {
		logger.Error("fetch metrics failed", "error", metricsErr)
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

func (s *Scheduler) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			snapshot := s.health.Snapshot()
			if err := s.pusher.PushHeartbeat(hbCtx, snapshot); err != nil {
				slog.Error("heartbeat failed", "error", err)
			}
			cancel()
		case <-ctx.Done():
			return
		}
	}
}
