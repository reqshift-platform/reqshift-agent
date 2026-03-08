package health

import (
	"fmt"
	"sync"
	"testing"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

func TestRecordSuccess(t *testing.T) {
	m := NewMonitor("test-agent")
	m.RecordSuccess("gravitee")

	snap := m.Snapshot()
	if snap.ConnectorStatus["gravitee"] != string(models.StatusHealthy) {
		t.Errorf("connector status = %q, want %q", snap.ConnectorStatus["gravitee"], models.StatusHealthy)
	}
	if snap.LastError != "" {
		t.Errorf("expected no last error, got %q", snap.LastError)
	}
}

func TestRecordSuccessClearsError(t *testing.T) {
	m := NewMonitor("test-agent")

	m.RecordError("gravitee", fmt.Errorf("connection refused"))
	snap := m.Snapshot()
	if snap.ConnectorStatus["gravitee"] != string(models.StatusError) {
		t.Errorf("expected error status after RecordError")
	}

	m.RecordSuccess("gravitee")
	snap = m.Snapshot()
	if snap.ConnectorStatus["gravitee"] != string(models.StatusHealthy) {
		t.Errorf("expected healthy status after RecordSuccess, got %q", snap.ConnectorStatus["gravitee"])
	}
	if snap.LastError != "" {
		t.Errorf("expected error cleared after RecordSuccess, got %q", snap.LastError)
	}
}

func TestRecordError(t *testing.T) {
	m := NewMonitor("test-agent")
	m.RecordError("kong", fmt.Errorf("timeout"))

	snap := m.Snapshot()
	if snap.ConnectorStatus["kong"] != string(models.StatusError) {
		t.Errorf("connector status = %q, want %q", snap.ConnectorStatus["kong"], models.StatusError)
	}
	if snap.LastError == "" {
		t.Error("expected last error to be set")
	}
}

func TestSnapshotHealthyWhenAllOK(t *testing.T) {
	m := NewMonitor("test-agent")
	m.RecordSuccess("gravitee")
	m.RecordSuccess("kong")

	snap := m.Snapshot()
	if snap.Status != models.StatusHealthy {
		t.Errorf("status = %q, want %q", snap.Status, models.StatusHealthy)
	}
}

func TestSnapshotDegradedIfAnyError(t *testing.T) {
	m := NewMonitor("test-agent")
	m.RecordSuccess("gravitee")
	m.RecordError("kong", fmt.Errorf("down"))

	snap := m.Snapshot()
	if snap.Status != models.StatusDegraded {
		t.Errorf("status = %q, want %q", snap.Status, models.StatusDegraded)
	}
}

func TestSnapshotUptimeAndMemory(t *testing.T) {
	m := NewMonitor("test-agent")
	snap := m.Snapshot()

	if snap.UptimeSeconds < 0 {
		t.Errorf("uptime should be >= 0, got %d", snap.UptimeSeconds)
	}
	if snap.MemoryUsedMB < 0 {
		t.Errorf("memory should be >= 0, got %d", snap.MemoryUsedMB)
	}
}

func TestSnapshotLastSyncAt(t *testing.T) {
	m := NewMonitor("test-agent")
	m.RecordSuccess("conn1")

	snap := m.Snapshot()
	if snap.LastSyncAt.IsZero() {
		t.Error("expected LastSyncAt to be set after RecordSuccess")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewMonitor("test-agent")
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(3)
		name := fmt.Sprintf("conn-%d", i%5)
		go func() {
			defer wg.Done()
			m.RecordSuccess(name)
		}()
		go func() {
			defer wg.Done()
			m.RecordError(name, fmt.Errorf("err"))
		}()
		go func() {
			defer wg.Done()
			_ = m.Snapshot()
		}()
	}

	wg.Wait()
	// If we get here without a panic, concurrent access is safe.
}
