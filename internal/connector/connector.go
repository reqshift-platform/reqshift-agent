package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// Connector is the interface all APIM connectors implement.
type Connector interface {
	Type() string
	Name() string
	FetchSpecs(ctx context.Context) ([]models.APISpec, error)
	FetchMetrics(ctx context.Context) ([]models.APIMetrics, error)
	HealthCheck(ctx context.Context) error
}

// Entry holds a connector alongside its sync interval.
type Entry struct {
	Connector    Connector
	SyncInterval time.Duration
}

// Factory creates a connector from config.
type Factory func(cfg config.ConnectorConfig) (Connector, error)

// Registry manages connector instances and factories.
type Registry struct {
	entries   []Entry
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// RegisterFactory adds a connector factory by type name.
func (r *Registry) RegisterFactory(typeName string, factory Factory) {
	r.factories[typeName] = factory
}

// Create instantiates a connector from config.
func (r *Registry) Create(cfg config.ConnectorConfig) (Connector, error) {
	factory, ok := r.factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown connector type: %s (registered: %v)",
			cfg.Type, r.FactoryNames())
	}
	return factory(cfg)
}

// Register adds a connector with its sync interval to the active set.
func (r *Registry) Register(c Connector, interval time.Duration) {
	r.entries = append(r.entries, Entry{Connector: c, SyncInterval: interval})
}

// All returns all registered connector entries.
func (r *Registry) All() []Entry {
	return r.entries
}

// FactoryNames returns the list of registered factory type names.
func (r *Registry) FactoryNames() []string {
	names := make([]string, 0, len(r.factories))
	for k := range r.factories {
		names = append(names, k)
	}
	return names
}
