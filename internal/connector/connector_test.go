package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// mockConnector implements the Connector interface for testing.
type mockConnector struct {
	typeName string
	name     string
}

func (m *mockConnector) Type() string                                                  { return m.typeName }
func (m *mockConnector) Name() string                                                  { return m.name }
func (m *mockConnector) FetchSpecs(_ context.Context) ([]models.APISpec, error)        { return nil, nil }
func (m *mockConnector) FetchMetrics(_ context.Context) ([]models.APIMetrics, error)   { return nil, nil }
func (m *mockConnector) HealthCheck(_ context.Context) error                           { return nil }

func TestRegisterFactoryAndCreate(t *testing.T) {
	r := NewRegistry()

	r.RegisterFactory("mock", func(cfg config.ConnectorConfig) (Connector, error) {
		return &mockConnector{typeName: "mock", name: cfg.Name}, nil
	})

	conn, err := r.Create(config.ConnectorConfig{Type: "mock", Name: "my-mock"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Type() != "mock" {
		t.Errorf("type = %q, want %q", conn.Type(), "mock")
	}
	if conn.Name() != "my-mock" {
		t.Errorf("name = %q, want %q", conn.Name(), "my-mock")
	}
}

func TestCreateUnknownType(t *testing.T) {
	r := NewRegistry()

	_, err := r.Create(config.ConnectorConfig{Type: "nonexistent"})
	if err == nil {
		t.Error("expected error for unknown connector type")
	}
	if err != nil && !strings.Contains(err.Error(), "unknown connector type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterAndAll(t *testing.T) {
	r := NewRegistry()

	c1 := &mockConnector{typeName: "a", name: "conn-1"}
	c2 := &mockConnector{typeName: "b", name: "conn-2"}

	r.Register(c1, 5*time.Minute)
	r.Register(c2, 10*time.Minute)

	entries := r.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Connector.Name() != "conn-1" {
		t.Errorf("first entry name = %q, want %q", entries[0].Connector.Name(), "conn-1")
	}
	if entries[0].SyncInterval != 5*time.Minute {
		t.Errorf("first entry interval = %v, want 5m", entries[0].SyncInterval)
	}
	if entries[1].Connector.Name() != "conn-2" {
		t.Errorf("second entry name = %q, want %q", entries[1].Connector.Name(), "conn-2")
	}
}

func TestFactoryNames(t *testing.T) {
	r := NewRegistry()
	r.RegisterFactory("gravitee", func(cfg config.ConnectorConfig) (Connector, error) { return nil, nil })
	r.RegisterFactory("kong", func(cfg config.ConnectorConfig) (Connector, error) { return nil, nil })
	r.RegisterFactory("openapi", func(cfg config.ConnectorConfig) (Connector, error) { return nil, nil })

	names := r.FactoryNames()
	sort.Strings(names)

	expected := []string{"gravitee", "kong", "openapi"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("name[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestCreateFactoryError(t *testing.T) {
	r := NewRegistry()
	r.RegisterFactory("broken", func(cfg config.ConnectorConfig) (Connector, error) {
		return nil, fmt.Errorf("factory failed")
	})

	_, err := r.Create(config.ConnectorConfig{Type: "broken"})
	if err == nil || !strings.Contains(err.Error(), "factory failed") {
		t.Errorf("expected factory error, got: %v", err)
	}
}

