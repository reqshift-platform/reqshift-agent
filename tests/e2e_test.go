// Package tests contains end-to-end regression tests that verify the full
// communication pipeline: Connector → Push Client → Cloud API.
//
// These tests use real components (no mocks) with a fake HTTP cloud server
// to catch regressions in the sync payload structure, headers, and format.
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/reqshift-platform/reqshift-agent/connectors/openapi"
	"github.com/reqshift-platform/reqshift-agent/connectors/traffic"
	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/internal/health"
	"github.com/reqshift-platform/reqshift-agent/internal/push"
	"github.com/reqshift-platform/reqshift-agent/internal/scheduler"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// capturedRequest stores an incoming HTTP request for assertions.
type capturedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// fakeCloud is an httptest server that records every request it receives.
type fakeCloud struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

func newFakeCloud() *fakeCloud {
	fc := &fakeCloud{}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fc.mu.Lock()
		fc.requests = append(fc.requests, capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    body,
		})
		fc.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if r.URL.Path == "/ingest/sync" {
			_ = json.NewEncoder(w).Encode(models.SyncResponse{
				Status:        "ok",
				SpecsIngested: 1,
				NextSyncIn:    300,
			})
		} else {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	return fc
}

func (fc *fakeCloud) syncRequests() []capturedRequest {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	var syncs []capturedRequest
	for _, r := range fc.requests {
		if r.Path == "/ingest/sync" {
			syncs = append(syncs, r)
		}
	}
	return syncs
}

func (fc *fakeCloud) heartbeatRequests() []capturedRequest {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	var hbs []capturedRequest
	for _, r := range fc.requests {
		if r.Path == "/ingest/heartbeat" {
			hbs = append(hbs, r)
		}
	}
	return hbs
}

func writeSpecFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// e2ePipeline holds all components for an e2e test.
type e2ePipeline struct {
	scheduler *scheduler.Scheduler
	monitor   *health.Monitor
}

// newOpenAPIPipeline creates a full pipeline with the openapi connector pointing at specDir.
func newOpenAPIPipeline(t *testing.T, cloudURL, agentID, connName, specDir string, syncInterval time.Duration) *e2ePipeline {
	t.Helper()
	reg := connector.NewRegistry()
	reg.RegisterFactory("openapi", func(cfg config.ConnectorConfig) (connector.Connector, error) {
		return openapi.NewConnector(cfg)
	})

	connCfg := config.ConnectorConfig{
		Type:         "openapi",
		Name:         connName,
		SyncInterval: syncInterval,
		Options:      map[string]string{"watch-dir": specDir},
	}
	conn, err := reg.Create(connCfg)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	reg.Register(conn, connCfg.SyncInterval)

	cfg := &config.Config{
		Agent:      config.AgentConfig{ID: agentID},
		Cloud:      config.CloudConfig{Endpoint: cloudURL, APIKey: "key"},
		Connectors: []config.ConnectorConfig{connCfg},
	}

	pusher := push.NewClient(cloudURL, "key", agentID, "test")
	mon := health.NewMonitor(agentID)
	sched := scheduler.New(reg, pusher, mon, cfg, "test")

	return &e2ePipeline{scheduler: sched, monitor: mon}
}

// runOneSyncCycle starts the scheduler, waits for initial sync, then stops.
func (p *e2ePipeline) runOneSyncCycle(t *testing.T) {
	t.Helper()
	p.scheduler.Start()
	time.Sleep(500 * time.Millisecond)
	p.scheduler.Stop()
}

// --------------------------------------------------------------------------
// E2E: OpenAPI connector → full sync → verify payload
// --------------------------------------------------------------------------

func TestE2E_OpenAPISync(t *testing.T) {
	specDir := t.TempDir()
	writeSpecFile(t, specDir, "petstore.json", `{
		"openapi": "3.0.0",
		"info": {"title": "Petstore", "version": "1.0.0"},
		"paths": {
			"/pets": {"get": {"summary": "List pets"}},
			"/pets/{id}": {"get": {"summary": "Get pet"}}
		}
	}`)
	writeSpecFile(t, specDir, "payments.yaml", `openapi: "3.0.1"
info:
  title: Payments API
  version: "2.0.0"
paths:
  /payments:
    post:
      summary: Create payment
`)

	cloud := newFakeCloud()
	defer cloud.Close()

	// Use custom pipeline for this test (needs specific apiKey/version for header checks)
	reg := connector.NewRegistry()
	reg.RegisterFactory("openapi", func(cfg config.ConnectorConfig) (connector.Connector, error) {
		return openapi.NewConnector(cfg)
	})
	connCfg := config.ConnectorConfig{
		Type:         "openapi",
		Name:         "e2e-specs",
		SyncInterval: 1 * time.Hour,
		Options:      map[string]string{"watch-dir": specDir},
	}
	conn, err := reg.Create(connCfg)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	reg.Register(conn, connCfg.SyncInterval)

	cfg := &config.Config{
		Agent: config.AgentConfig{ID: "e2e-agent", Name: "E2E Test"},
		Cloud: config.CloudConfig{Endpoint: cloud.URL, APIKey: "e2e-key"},
		Connectors: []config.ConnectorConfig{connCfg},
	}
	pusher := push.NewClient(cloud.URL, "e2e-key", "e2e-agent", "1.0.0-test")
	mon := health.NewMonitor("e2e-agent")
	sched := scheduler.New(reg, pusher, mon, cfg, "1.0.0-test")

	sched.Start()
	time.Sleep(500 * time.Millisecond)
	sched.Stop()

	syncs := cloud.syncRequests()
	if len(syncs) == 0 {
		t.Fatal("expected at least 1 sync request, got 0")
	}

	req := syncs[0]

	// --- Verify HTTP method & headers ---
	if req.Method != "POST" {
		t.Errorf("method = %q, want POST", req.Method)
	}
	if got := req.Headers.Get("X-Agent-Key"); got != "e2e-key" {
		t.Errorf("X-Agent-Key = %q, want %q", got, "e2e-key")
	}
	if got := req.Headers.Get("X-Agent-Id"); got != "e2e-agent" {
		t.Errorf("X-Agent-Id = %q, want %q", got, "e2e-agent")
	}
	if got := req.Headers.Get("User-Agent"); got != "reqshift-agent/1.0.0-test" {
		t.Errorf("User-Agent = %q, want %q", got, "reqshift-agent/1.0.0-test")
	}
	if got := req.Headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	// --- Parse payload ---
	var payload models.SyncPayload
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("unmarshal sync payload: %v", err)
	}

	// --- Verify payload metadata ---
	if payload.AgentID != "e2e-agent" {
		t.Errorf("agentId = %q, want %q", payload.AgentID, "e2e-agent")
	}
	if payload.AgentVersion != "1.0.0-test" {
		t.Errorf("agentVersion = %q, want %q", payload.AgentVersion, "1.0.0-test")
	}
	if payload.ConnectorType != "openapi" {
		t.Errorf("connectorType = %q, want %q", payload.ConnectorType, "openapi")
	}
	if payload.ConnectorName != "e2e-specs" {
		t.Errorf("connectorName = %q, want %q", payload.ConnectorName, "e2e-specs")
	}
	if payload.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}

	// --- Verify specs discovered ---
	if len(payload.Specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(payload.Specs))
	}

	specsByName := map[string]models.APISpec{}
	for _, s := range payload.Specs {
		specsByName[s.APIName] = s
	}

	pet, ok := specsByName["petstore"]
	if !ok {
		t.Fatal("petstore spec not found")
	}
	if pet.APIID != "file:petstore.json" {
		t.Errorf("petstore APIID = %q", pet.APIID)
	}
	if pet.SpecFormat != models.SpecOpenAPI3 {
		t.Errorf("petstore format = %q, want openapi3", pet.SpecFormat)
	}
	if pet.SpecContent == "" {
		t.Error("petstore specContent is empty")
	}
	if pet.LastModified.IsZero() {
		t.Error("petstore lastModified is zero")
	}

	pay, ok := specsByName["payments"]
	if !ok {
		t.Fatal("payments spec not found")
	}
	if pay.SpecFormat != models.SpecOpenAPI3 {
		t.Errorf("payments format = %q, want openapi3", pay.SpecFormat)
	}

	if payload.Health == nil {
		t.Error("health should be present in sync payload")
	} else if payload.Health.Status != models.StatusHealthy {
		t.Errorf("health status = %q, want healthy", payload.Health.Status)
	}
}

// --------------------------------------------------------------------------
// E2E: Verify heartbeat is sent
// --------------------------------------------------------------------------

func TestE2E_HeartbeatSent(t *testing.T) {
	specDir := t.TempDir()
	writeSpecFile(t, specDir, "api.json", `{"openapi": "3.0.0"}`)

	cloud := newFakeCloud()
	defer cloud.Close()

	p := newOpenAPIPipeline(t, cloud.URL, "hb-agent", "hb-test", specDir, 1*time.Hour)

	// Heartbeat interval is 30s — we wait enough time for at least one
	p.scheduler.Start()
	time.Sleep(31 * time.Second)
	p.scheduler.Stop()

	hbs := cloud.heartbeatRequests()
	if len(hbs) == 0 {
		t.Fatal("expected at least 1 heartbeat, got 0")
	}

	var h models.AgentHealth
	if err := json.Unmarshal(hbs[0].Body, &h); err != nil {
		t.Fatalf("unmarshal heartbeat: %v", err)
	}
	if h.Status != models.StatusHealthy {
		t.Errorf("heartbeat status = %q, want healthy", h.Status)
	}
	if h.UptimeSeconds <= 0 {
		t.Errorf("uptime should be > 0, got %d", h.UptimeSeconds)
	}
}

// --------------------------------------------------------------------------
// E2E: Cloud down → health degraded → cloud back → recovery
// --------------------------------------------------------------------------

func TestE2E_CloudDownThenRecovery(t *testing.T) {
	specDir := t.TempDir()
	writeSpecFile(t, specDir, "api.json", `{"openapi": "3.0.0"}`)

	// Start with cloud DOWN (returns 500)
	var cloudUp bool
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		up := cloudUp
		mu.Unlock()

		if !up {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("cloud is down"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/ingest/sync" {
			_ = json.NewEncoder(w).Encode(models.SyncResponse{Status: "ok", SpecsIngested: 1})
		} else {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	p := newOpenAPIPipeline(t, server.URL, "res-agent", "resilience-test", specDir, 1*time.Second)

	// Phase 1: Cloud is down
	p.scheduler.Start()
	time.Sleep(5 * time.Second)

	snap := p.monitor.Snapshot()
	if snap.ConnectorStatus["cloud"] != string(models.StatusError) {
		t.Errorf("phase 1: cloud status = %q, want error", snap.ConnectorStatus["cloud"])
	}

	// Phase 2: Bring cloud back up
	mu.Lock()
	cloudUp = true
	mu.Unlock()

	time.Sleep(5 * time.Second)

	snap = p.monitor.Snapshot()
	if snap.ConnectorStatus["resilience-test"] != string(models.StatusHealthy) {
		t.Errorf("phase 2: connector status = %q, want healthy", snap.ConnectorStatus["resilience-test"])
	}

	p.scheduler.Stop()
}

// --------------------------------------------------------------------------
// E2E: Traffic logs connector → verify parsed entries arrive
// --------------------------------------------------------------------------

func TestE2E_TrafficLogsSync(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "access.log")
	lines := `192.168.1.10 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/users HTTP/1.1" 200 1234 "-" "curl/7.88" 0.042
192.168.1.20 - - [10/Oct/2024:13:55:37 -0700] "POST /api/v1/orders HTTP/1.1" 201 567 "-" "python/3.11" 0.125
10.0.0.5 - - [10/Oct/2024:13:55:38 -0700] "GET /health HTTP/1.1" 200 2 "-" "kube-probe" 0.001
`
	if err := os.WriteFile(logFile, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	cloud := newFakeCloud()
	defer cloud.Close()

	reg := connector.NewRegistry()
	reg.RegisterFactory("traffic-logs", func(cfg config.ConnectorConfig) (connector.Connector, error) {
		return traffic.NewConnector(cfg)
	})

	connCfg := config.ConnectorConfig{
		Type:         "traffic-logs",
		Name:         "e2e-traffic",
		SyncInterval: 1 * time.Hour,
		Options: map[string]string{
			"log-path":    logFile,
			"sample-rate": "1.0",
		},
	}
	conn, err := reg.Create(connCfg)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	reg.Register(conn, connCfg.SyncInterval)

	cfg := &config.Config{
		Agent:      config.AgentConfig{ID: "traffic-agent"},
		Cloud:      config.CloudConfig{Endpoint: cloud.URL, APIKey: "key"},
		Connectors: []config.ConnectorConfig{connCfg},
	}

	pusher := push.NewClient(cloud.URL, "key", "traffic-agent", "test")
	mon := health.NewMonitor("traffic-agent")
	sched := scheduler.New(reg, pusher, mon, cfg, "test")

	sched.Start()
	time.Sleep(500 * time.Millisecond)
	sched.Stop()

	syncs := cloud.syncRequests()
	if len(syncs) == 0 {
		t.Fatal("expected at least 1 sync request")
	}

	var payload models.SyncPayload
	if err := json.Unmarshal(syncs[0].Body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.ConnectorType != "traffic-logs" {
		t.Errorf("connectorType = %q, want traffic-logs", payload.ConnectorType)
	}
	if payload.ConnectorName != "e2e-traffic" {
		t.Errorf("connectorName = %q, want e2e-traffic", payload.ConnectorName)
	}
	if payload.Health == nil {
		t.Error("health should be present")
	}
}

// --------------------------------------------------------------------------
// E2E: Payload schema stability (regression guard)
// --------------------------------------------------------------------------

func TestE2E_PayloadSchemaStability(t *testing.T) {
	specDir := t.TempDir()
	writeSpecFile(t, specDir, "test.json", `{"openapi": "3.0.0", "info": {"title": "Schema Test"}}`)

	cloud := newFakeCloud()
	defer cloud.Close()

	p := newOpenAPIPipeline(t, cloud.URL, "schema-agent", "schema-test", specDir, 1*time.Hour)
	p.runOneSyncCycle(t)

	syncs := cloud.syncRequests()
	if len(syncs) == 0 {
		t.Fatal("no sync received")
	}

	// Parse as raw JSON map to verify ALL expected fields exist
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(syncs[0].Body, &raw); err != nil {
		t.Fatal(err)
	}

	requiredTopLevel := []string{
		"agentId", "agentVersion", "timestamp",
		"connectorType", "connectorName", "specs", "health",
	}
	for _, field := range requiredTopLevel {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required top-level field %q in sync payload", field)
		}
	}

	// Verify spec fields
	var specs []map[string]json.RawMessage
	if err := json.Unmarshal(raw["specs"], &specs); err != nil {
		t.Fatal(err)
	}
	if len(specs) == 0 {
		t.Fatal("no specs in payload")
	}

	requiredSpecFields := []string{"apiId", "apiName", "specFormat", "specContent", "lastModified"}
	for _, field := range requiredSpecFields {
		if _, ok := specs[0][field]; !ok {
			t.Errorf("missing required spec field %q", field)
		}
	}

	// Verify health fields
	var healthRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw["health"], &healthRaw); err != nil {
		t.Fatal(err)
	}

	requiredHealthFields := []string{"status", "uptimeSeconds", "memoryUsedMb", "lastSyncAt"}
	for _, field := range requiredHealthFields {
		if _, ok := healthRaw[field]; !ok {
			t.Errorf("missing required health field %q", field)
		}
	}
}
