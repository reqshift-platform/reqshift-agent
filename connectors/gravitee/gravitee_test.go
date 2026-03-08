package gravitee

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
)

func TestNewConnectorMissingURL(t *testing.T) {
	_, err := NewConnector(config.ConnectorConfig{
		Name: "gravitee-test",
	})
	if err == nil || !strings.Contains(err.Error(), "requires url") {
		t.Errorf("expected url required error, got: %v", err)
	}
}

func TestNewConnectorDefaultEnv(t *testing.T) {
	conn, err := NewConnector(config.ConnectorConfig{
		Name: "gravitee-test",
		URL:  "http://localhost:8083",
		Auth: config.AuthConfig{Token: "token"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := conn.(*Connector)
	if g.envID != "DEFAULT" {
		t.Errorf("envID = %q, want %q", g.envID, "DEFAULT")
	}
}

func TestNewConnectorCustomEnv(t *testing.T) {
	conn, err := NewConnector(config.ConnectorConfig{
		Name:    "gravitee-test",
		URL:     "http://localhost:8083",
		Auth:    config.AuthConfig{Token: "token"},
		Options: map[string]string{"environment": "production"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := conn.(*Connector)
	if g.envID != "production" {
		t.Errorf("envID = %q, want %q", g.envID, "production")
	}
}

func TestTypeAndName(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "my-gravitee",
		URL:  "http://localhost:8083",
	})
	if conn.Type() != "gravitee" {
		t.Errorf("Type() = %q, want %q", conn.Type(), "gravitee")
	}
	if conn.Name() != "my-gravitee" {
		t.Errorf("Name() = %q, want %q", conn.Name(), "my-gravitee")
	}
}

func TestFetchSpecs(t *testing.T) {
	apiListResp := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"id":          "api-001",
				"name":        "Payment API",
				"apiVersion":  "v1",
				"contextPath": "/payments",
				"tags":        []string{"finance"},
				"updatedAt":   time.Now().Format(time.RFC3339),
			},
			{
				"id":         "api-002",
				"name":       "Users API",
				"apiVersion": "v2",
				"updatedAt":  time.Now().Format(time.RFC3339),
			},
		},
	}

	specContent := `{"openapi": "3.0.0", "info": {"title": "Payment API"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer auth, got %q", auth)
		}

		if strings.HasSuffix(r.URL.Path, "/apis") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(apiListResp)
		} else if strings.Contains(r.URL.Path, "/definition") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(specContent))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "my-token"},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].APIID != "api-001" {
		t.Errorf("first spec APIID = %q, want %q", specs[0].APIID, "api-001")
	}
	if specs[0].APIName != "Payment API" {
		t.Errorf("first spec APIName = %q", specs[0].APIName)
	}
	if specs[0].BasePath != "/payments" {
		t.Errorf("first spec BasePath = %q", specs[0].BasePath)
	}
	if specs[0].SpecFormat != "openapi3" {
		t.Errorf("first spec SpecFormat = %q, want openapi3", specs[0].SpecFormat)
	}
}

func TestFetchMetrics(t *testing.T) {
	analyticsResp := map[string]interface{}{
		"values": map[string]interface{}{
			"api-001": map[string]interface{}{
				"hits": 1500,
				"avg":  42.5,
				"p95":  120.0,
				"p99":  250.0,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/analytics") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(analyticsResp)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "token"},
	})

	metrics, err := conn.FetchMetrics(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].APIID != "api-001" {
		t.Errorf("APIID = %q, want %q", metrics[0].APIID, "api-001")
	}
	if metrics[0].RequestCount != 1500 {
		t.Errorf("RequestCount = %d, want 1500", metrics[0].RequestCount)
	}
	if metrics[0].LatencyP50Ms != 42.5 {
		t.Errorf("LatencyP50Ms = %f, want 42.5", metrics[0].LatencyP50Ms)
	}
	if metrics[0].LatencyP95Ms != 120.0 {
		t.Errorf("LatencyP95Ms = %f, want 120.0", metrics[0].LatencyP95Ms)
	}
}

func TestHealthCheckOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "token"},
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
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "token"},
	})

	err := conn.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for 503 response")
	}
}

func TestAuthBearerHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer secret-token")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "secret-token"},
	})

	_ = conn.HealthCheck(context.Background())
}

func TestFetchSpecsAPIListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name: "test-gravitee",
		URL:  server.URL,
		Auth: config.AuthConfig{Token: "token"},
	})

	_, err := conn.FetchSpecs(context.Background())
	if err == nil {
		t.Error("expected error for 500 response")
	}
}
