package openapi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

var _ connector.Connector = (*Connector)(nil)

type Connector struct {
	name     string
	watchDir string
}

func NewConnector(cfg config.ConnectorConfig) (connector.Connector, error) {
	dir := cfg.Options["watch-dir"]
	if dir == "" {
		dir = cfg.URL
	}
	if dir == "" {
		return nil, fmt.Errorf("openapi connector requires options.watch-dir")
	}
	return &Connector{name: cfg.Name, watchDir: dir}, nil
}

func (o *Connector) Type() string { return "openapi" }
func (o *Connector) Name() string { return o.name }

func (o *Connector) FetchSpecs(_ context.Context) ([]models.APISpec, error) {
	var specs []models.APISpec

	err := filepath.Walk(o.watchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		name := strings.TrimSuffix(filepath.Base(path), ext)

		specs = append(specs, models.APISpec{
			APIID:        fmt.Sprintf("file:%s", filepath.Base(path)),
			APIName:      name,
			SpecFormat:   models.DetectSpecFormat(string(content)),
			SpecContent:  string(content),
			LastModified: info.ModTime(),
		})

		return nil
	})

	return specs, err
}

func (o *Connector) FetchMetrics(_ context.Context) ([]models.APIMetrics, error) {
	return nil, nil
}

func (o *Connector) HealthCheck(_ context.Context) error {
	info, err := os.Stat(o.watchDir)
	if err != nil {
		return fmt.Errorf("watch dir not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("watch path is not a directory: %s", o.watchDir)
	}
	return nil
}

