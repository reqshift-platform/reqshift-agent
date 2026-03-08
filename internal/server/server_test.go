package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reqshift-platform/reqshift-agent/internal/health"
)

func TestHealthzReturns200(t *testing.T) {
	mon := health.NewMonitor("test")
	defer mon.Stop()

	srv := New(":0", mon, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp healthzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", resp.Version, "1.0.0")
	}
	if resp.Uptime < 0 {
		t.Error("uptime should be >= 0")
	}
}

func TestReadyzNoConnectors(t *testing.T) {
	mon := health.NewMonitor("test")
	defer mon.Stop()

	srv := New(":0", mon, "1.0.0")

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	// No connectors reported yet — ready is false but status is 200 (empty map).
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Ready {
		t.Error("expected not ready with no connectors")
	}
}

func TestReadyzWithHealthyConnector(t *testing.T) {
	mon := health.NewMonitor("test")
	defer mon.Stop()
	mon.RecordSuccess("my-connector")

	srv := New(":0", mon, "1.0.0")

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Ready {
		t.Error("expected ready with healthy connector")
	}
}

func TestReadyzAllUnhealthy(t *testing.T) {
	mon := health.NewMonitor("test")
	defer mon.Stop()
	mon.RecordError("conn-1", errForTest("fail"))

	srv := New(":0", mon, "1.0.0")

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Ready {
		t.Error("expected not ready with all unhealthy connectors")
	}
}

func TestStartStop(t *testing.T) {
	mon := health.NewMonitor("test")
	defer mon.Stop()

	srv := New("127.0.0.1:0", mon, "1.0.0")
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := srv.Stop(t.Context()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

type testError string

func errForTest(msg string) testError { return testError(msg) }
func (e testError) Error() string     { return string(e) }
