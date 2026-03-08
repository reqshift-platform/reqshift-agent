package health

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

const memStatsInterval = 10 * time.Second

// Monitor tracks the agent's own health and per-connector status.
type Monitor struct {
	agentID   string
	startedAt time.Time

	mu              sync.RWMutex
	connectorStatus map[string]models.HealthStatus
	connectorErrors map[string]string
	lastSyncAt      time.Time

	cachedAllocMB atomic.Int64
	stopMem       chan struct{}
}

func NewMonitor(agentID string) *Monitor {
	m := &Monitor{
		agentID:         agentID,
		startedAt:       time.Now(),
		connectorStatus: make(map[string]models.HealthStatus),
		connectorErrors: make(map[string]string),
		stopMem:         make(chan struct{}),
	}

	// Seed initial value.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.cachedAllocMB.Store(int64(ms.Alloc / 1024 / 1024))

	go m.refreshMemStats()
	return m
}

func (m *Monitor) refreshMemStats() {
	ticker := time.NewTicker(memStatsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			m.cachedAllocMB.Store(int64(ms.Alloc / 1024 / 1024))
		case <-m.stopMem:
			return
		}
	}
}

// Stop shuts down the background mem-stats goroutine.
func (m *Monitor) Stop() {
	close(m.stopMem)
}

// RecordSuccess marks a connector as healthy.
func (m *Monitor) RecordSuccess(connectorName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectorStatus[connectorName] = models.StatusHealthy
	delete(m.connectorErrors, connectorName)
	m.lastSyncAt = time.Now()
}

// RecordError marks a connector as errored.
func (m *Monitor) RecordError(connectorName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectorStatus[connectorName] = models.StatusError
	m.connectorErrors[connectorName] = err.Error()
}

// Snapshot returns the current health state.
func (m *Monitor) Snapshot() *models.AgentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := models.StatusHealthy
	var lastError string
	for _, s := range m.connectorStatus {
		if s == models.StatusError {
			status = models.StatusDegraded
			break
		}
	}

	// Pick the first connector error as the reported lastError.
	for _, e := range m.connectorErrors {
		lastError = e
		break
	}

	connStatus := make(map[string]string, len(m.connectorStatus))
	for k, v := range m.connectorStatus {
		connStatus[k] = string(v)
	}

	return &models.AgentHealth{
		Status:          status,
		UptimeSeconds:   int64(time.Since(m.startedAt).Seconds()),
		MemoryUsedMB:    m.cachedAllocMB.Load(),
		LastSyncAt:      m.lastSyncAt,
		LastError:       lastError,
		ConnectorStatus: connStatus,
	}
}
