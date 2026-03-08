package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

func TestPushSyncSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ingest/sync" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(models.SyncResponse{
			Status:          "ok",
			SpecsIngested:   3,
			MetricsIngested: 2,
			NextSyncIn:      300,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	resp, err := client.PushSync(context.Background(), &models.SyncPayload{
		AgentID:       "agent-01",
		AgentVersion:  "1.0.0",
		Timestamp:     time.Now(),
		ConnectorType: "openapi",
		ConnectorName: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.SpecsIngested != 3 {
		t.Errorf("specsIngested = %d, want 3", resp.SpecsIngested)
	}
	if resp.MetricsIngested != 2 {
		t.Errorf("metricsIngested = %d, want 2", resp.MetricsIngested)
	}
}

func TestPushSyncServerError(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	_, err := client.PushSync(context.Background(), &models.SyncPayload{})
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("unexpected error: %v", err)
	}
	// Verify retry happened (3 attempts for 5xx)
	if attempts != maxRetries {
		t.Errorf("expected %d attempts, got %d", maxRetries, attempts)
	}
}

func TestPushSyncClientErrorNoRetry(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	_, err := client.PushSync(context.Background(), &models.SyncPayload{})
	if err == nil {
		t.Error("expected error for 400 response")
	}
	// 4xx should NOT be retried
	if attempts != 1 {
		t.Errorf("expected 1 attempt for 4xx, got %d", attempts)
	}
}

func TestPushSyncRetryThenSuccess(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary error"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(models.SyncResponse{Status: "ok", SpecsIngested: 1})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	resp, err := client.PushSync(context.Background(), &models.SyncPayload{})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPushHeartbeatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ingest/heartbeat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	err := client.PushHeartbeat(context.Background(), &models.AgentHealth{
		Status: models.StatusHealthy,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPushHeartbeatTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "agent-01", "1.0.0")
	client.httpClient.Timeout = 100 * time.Millisecond

	// Use a context with short deadline to avoid waiting for all retries
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := client.PushHeartbeat(ctx, &models.AgentHealth{})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestRequestHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Agent-Key"); got != "my-api-key" {
			t.Errorf("X-Agent-Key = %q, want %q", got, "my-api-key")
		}
		if got := r.Header.Get("X-Agent-Id"); got != "agent-42" {
			t.Errorf("X-Agent-Id = %q, want %q", got, "agent-42")
		}
		if got := r.Header.Get("User-Agent"); got != "reqshift-agent/2.0.0" {
			t.Errorf("User-Agent = %q, want %q", got, "reqshift-agent/2.0.0")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-api-key", "agent-42", "2.0.0")
	_ = client.PushHeartbeat(context.Background(), &models.AgentHealth{})
}

func TestRequestBodyContainsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload models.SyncPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("failed to unmarshal body: %v", err)
		}
		if payload.AgentID != "agent-01" {
			t.Errorf("agentId = %q, want %q", payload.AgentID, "agent-01")
		}
		if payload.ConnectorType != "kong" {
			t.Errorf("connectorType = %q, want %q", payload.ConnectorType, "kong")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(models.SyncResponse{Status: "ok"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key", "agent-01", "1.0.0")
	_, _ = client.PushSync(context.Background(), &models.SyncPayload{
		AgentID:       "agent-01",
		ConnectorType: "kong",
	})
}

func TestPushSyncContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "key", "agent-01", "1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.PushSync(ctx, &models.SyncPayload{})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
