package openapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
)

func TestNewConnectorMissingDir(t *testing.T) {
	_, err := NewConnector(config.ConnectorConfig{
		Name: "test-openapi",
	})
	if err == nil || !strings.Contains(err.Error(), "requires options.watch-dir") {
		t.Errorf("expected watch-dir error, got: %v", err)
	}
}

func TestNewConnectorFromOptions(t *testing.T) {
	dir := t.TempDir()
	conn, err := NewConnector(config.ConnectorConfig{
		Name:    "test-openapi",
		Options: map[string]string{"watch-dir": dir},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Name() != "test-openapi" {
		t.Errorf("Name() = %q", conn.Name())
	}
	if conn.Type() != "openapi" {
		t.Errorf("Type() = %q", conn.Type())
	}
}

func TestNewConnectorFromURL(t *testing.T) {
	dir := t.TempDir()
	conn, err := NewConnector(config.ConnectorConfig{
		Name: "test-openapi",
		URL:  dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o := conn.(*Connector)
	if o.watchDir != dir {
		t.Errorf("watchDir = %q, want %q", o.watchDir, dir)
	}
}

func TestFetchSpecs(t *testing.T) {
	dir := t.TempDir()

	// Create test spec files
	writeFile(t, dir, "petstore.json", `{"openapi": "3.0.0", "info": {"title": "Petstore"}}`)
	writeFile(t, dir, "users.yaml", "openapi: 3.0.0\ninfo:\n  title: Users")
	writeFile(t, dir, "events.yml", "asyncapi: 2.6.0\ninfo:\n  title: Events")
	writeFile(t, dir, "readme.txt", "not a spec") // Should be skipped

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": dir},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}

	// Verify spec content by finding each file
	found := map[string]bool{}
	for _, s := range specs {
		found[s.APIName] = true
		if s.APIID == "" {
			t.Error("APIID should not be empty")
		}
		if s.SpecContent == "" {
			t.Error("SpecContent should not be empty")
		}
	}

	for _, name := range []string{"petstore", "users", "events"} {
		if !found[name] {
			t.Errorf("expected to find spec named %q", name)
		}
	}
}

func TestFetchSpecsFormatDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "oas3.json", `{"openapi": "3.0.0"}`)
	writeFile(t, dir, "swagger.json", `{"swagger": "2.0"}`)
	writeFile(t, dir, "async.yaml", "asyncapi: 2.6.0")
	writeFile(t, dir, "unknown.json", `{"type": "other"}`)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": dir},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	formats := map[string]string{}
	for _, s := range specs {
		formats[s.APIName] = string(s.SpecFormat)
	}

	expected := map[string]string{
		"oas3":    "openapi3",
		"swagger": "openapi2",
		"async":   "asyncapi",
		"unknown": "unknown",
	}
	for name, want := range expected {
		if got := formats[name]; got != want {
			t.Errorf("format for %q = %q, want %q", name, got, want)
		}
	}
}

func TestFetchSpecsRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "root.json", `{"openapi": "3.0.0"}`)
	writeFile(t, sub, "nested.yaml", "openapi: 3.0.0")

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": dir},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Errorf("expected 2 specs (including nested), got %d", len(specs))
	}
}

func TestFetchSpecsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": dir},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for empty dir, got %d", len(specs))
	}
}

func TestFetchMetricsNil(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": t.TempDir()},
	})
	metrics, err := conn.FetchMetrics(context.Background())
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if metrics != nil {
		t.Errorf("expected nil metrics")
	}
}

func TestHealthCheckExistingDir(t *testing.T) {
	dir := t.TempDir()
	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": dir},
	})

	err := conn.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestHealthCheckNonexistentDir(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": "/nonexistent/path/specs"},
	})

	err := conn.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestHealthCheckFileNotDir(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "test",
		Options: map[string]string{"watch-dir": filePath},
	})

	err := conn.HealthCheck(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' error, got: %v", err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
