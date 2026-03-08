package kong

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
)

func TestNewConnectorMissingURL(t *testing.T) {
	_, err := NewConnector(config.ConnectorConfig{
		Name: "kong-test",
	})
	if err == nil || !strings.Contains(err.Error(), "requires url") {
		t.Errorf("expected url required error, got: %v", err)
	}
}

func TestTypeAndName(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "my-kong",
		URL:  "http://localhost:8001",
	})
	if conn.Type() != "kong" {
		t.Errorf("Type() = %q, want %q", conn.Type(), "kong")
	}
	if conn.Name() != "my-kong" {
		t.Errorf("Name() = %q, want %q", conn.Name(), "my-kong")
	}
}

func TestFetchSpecs(t *testing.T) {
	servicesResp := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"id":       "svc-001",
				"name":     "payment-service",
				"path":     "/v1/payments",
				"protocol": "https",
				"host":     "payment.internal",
			},
			{
				"id":       "svc-002",
				"name":     "user-service",
				"path":     "/v1/users",
				"protocol": "http",
				"host":     "user.internal",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(servicesResp)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].APIID != "svc-001" {
		t.Errorf("APIID = %q, want %q", specs[0].APIID, "svc-001")
	}
	if specs[0].APIName != "payment-service" {
		t.Errorf("APIName = %q", specs[0].APIName)
	}
	if specs[0].BasePath != "/v1/payments" {
		t.Errorf("BasePath = %q", specs[0].BasePath)
	}
	if specs[0].Metadata["protocol"] != "https" {
		t.Errorf("protocol = %q", specs[0].Metadata["protocol"])
	}
	if specs[0].Metadata["host"] != "payment.internal" {
		t.Errorf("host = %q", specs[0].Metadata["host"])
	}
}

func TestFetchMetricsNil(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  "http://localhost:8001",
	})
	metrics, err := conn.FetchMetrics(context.Background())
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if metrics != nil {
		t.Errorf("expected nil metrics, got %v", metrics)
	}
}

func TestHealthCheckOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server":{"connections_active":10}}`))
		}
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
	})

	err := conn.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestHealthCheckError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
	})

	err := conn.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for 503 response")
	}
}

func TestAuthHeaderOptional(t *testing.T) {
	// Without auth token — no Kong-Admin-Token header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Kong-Admin-Token") != "" {
			t.Error("expected no Kong-Admin-Token when auth not configured")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
	})
	_ = conn.HealthCheck(context.Background())
}

func TestAuthHeaderPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Kong-Admin-Token"); got != "my-admin-token" {
			t.Errorf("Kong-Admin-Token = %q, want %q", got, "my-admin-token")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "my-admin-token"},
	})
	_ = conn.HealthCheck(context.Background())
}

func TestFetchSpecsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-kong",
		URL:  server.URL,
	})

	_, err := conn.FetchSpecs(context.Background())
	if err == nil {
		t.Error("expected error for 500 response")
	}
}
