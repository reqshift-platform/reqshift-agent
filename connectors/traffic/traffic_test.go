package traffic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
)

func TestNewConnectorMissingLogPath(t *testing.T) {
	_, err := NewConnector(config.ConnectorConfig{
		Name: "traffic-test",
	})
	if err == nil || !strings.Contains(err.Error(), "requires options.log-path") {
		t.Errorf("expected log-path error, got: %v", err)
	}
}

func TestNewConnectorDefaultSampleRate(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, err := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := conn.(*Connector)
	if c.sampleRate != 0.1 {
		t.Errorf("sampleRate = %f, want 0.1", c.sampleRate)
	}
}

func TestNewConnectorCustomSampleRate(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, err := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile, "sample-rate": "0.5"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := conn.(*Connector)
	if c.sampleRate != 0.5 {
		t.Errorf("sampleRate = %f, want 0.5", c.sampleRate)
	}
}

func TestNewConnectorInvalidSampleRate(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile, "sample-rate": "abc"},
	})
	c := conn.(*Connector)
	// Should fall back to default 0.1
	if c.sampleRate != 0.1 {
		t.Errorf("sampleRate = %f, want 0.1 for invalid input", c.sampleRate)
	}
}

func TestTypeAndName(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "my-traffic",
		Options: map[string]string{"log-path": logFile},
	})
	if conn.Type() != "traffic-logs" {
		t.Errorf("Type() = %q, want %q", conn.Type(), "traffic-logs")
	}
	if conn.Name() != "my-traffic" {
		t.Errorf("Name() = %q, want %q", conn.Name(), "my-traffic")
	}
}

func TestParseLine(t *testing.T) {
	c := &Connector{}

	tests := []struct {
		name      string
		line      string
		wantErr   bool
		method    string
		path      string
		status    int
		hasLatency bool
	}{
		{
			name:      "Valid Nginx line with latency",
			line:      `192.168.1.100 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/users HTTP/1.1" 200 1234 "-" "curl/7.88" 0.042`,
			method:    "GET",
			path:      "/api/v1/users",
			status:    200,
			hasLatency: true,
		},
		{
			name:   "Valid line without latency",
			line:   `10.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "POST /api/orders HTTP/1.1" 201 567`,
			method: "POST",
			path:   "/api/orders",
			status: 201,
		},
		{
			name:   "Path with query string stripped",
			line:   `10.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/search?q=test HTTP/1.1" 200 100`,
			method: "GET",
			path:   "/api/search",
			status: 200,
		},
		{
			name:    "Malformed line",
			line:    "this is not a log line",
			wantErr: true,
		},
		{
			name:    "Empty line",
			line:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := c.parseLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Method != tt.method {
				t.Errorf("Method = %q, want %q", entry.Method, tt.method)
			}
			if entry.Path != tt.path {
				t.Errorf("Path = %q, want %q", entry.Path, tt.path)
			}
			if entry.Status != tt.status {
				t.Errorf("Status = %d, want %d", entry.Status, tt.status)
			}
			if tt.hasLatency && entry.LatencyMs <= 0 {
				t.Errorf("expected positive latency, got %f", entry.LatencyMs)
			}
		})
	}
}

func TestAnonymizeIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.100", "192.168.1.0"},
		{"10.0.0.255", "10.0.0.0"},
		{"127.0.0.1", "127.0.0.0"},
		{"::1", "::1"},                         // IPv6 unchanged
		{"fe80::1", "fe80::1"},                 // IPv6 unchanged
		{"not-an-ip", "not-an-ip"},             // Not an IP, unchanged
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := anonymizeIP(tt.input)
			if got != tt.expected {
				t.Errorf("anonymizeIP(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFetchTrafficEntries(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")

	lines := []string{
		`192.168.1.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/health HTTP/1.1" 200 100 "-" "curl" 0.001`,
		`192.168.1.2 - - [10/Oct/2024:13:55:37 -0700] "POST /api/v1/users HTTP/1.1" 201 200 "-" "curl" 0.050`,
		`192.168.1.3 - - [10/Oct/2024:13:55:38 -0700] "GET /api/v1/orders HTTP/1.1" 200 300 "-" "curl" 0.020`,
	}
	content := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(logFile, []byte(content), 0644)

	// Use sample rate 1.0 to capture all entries
	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile, "sample-rate": "1.0"},
	})

	tc := conn.(*Connector)
	entries, err := tc.FetchTrafficEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify IPs are anonymized
	for _, e := range entries {
		if !strings.HasSuffix(e.SourceIP, ".0") {
			t.Errorf("expected anonymized IP, got %q", e.SourceIP)
		}
	}
}

func TestFetchTrafficEntriesOffsetTracking(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")

	line1 := `10.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /first HTTP/1.1" 200 100 "-" "curl" 0.001`
	_ = os.WriteFile(logFile, []byte(line1+"\n"), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile, "sample-rate": "1.0"},
	})
	tc := conn.(*Connector)

	// First read
	entries1, _ := tc.FetchTrafficEntries()
	if len(entries1) != 1 {
		t.Fatalf("first read: expected 1 entry, got %d", len(entries1))
	}

	// Append new line
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	line2 := `10.0.0.2 - - [10/Oct/2024:13:56:00 -0700] "POST /second HTTP/1.1" 201 50 "-" "curl" 0.002`
	_, _ = f.WriteString(line2 + "\n")
	_ = f.Close()

	// Second read — should only get the new line
	entries2, _ := tc.FetchTrafficEntries()
	if len(entries2) != 1 {
		t.Fatalf("second read: expected 1 entry, got %d", len(entries2))
	}
	if entries2[0].Path != "/second" {
		t.Errorf("expected path /second, got %q", entries2[0].Path)
	}
}

func TestFetchSpecsAndMetricsNil(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile},
	})

	specs, err := conn.FetchSpecs(context.Background())
	if err != nil || specs != nil {
		t.Errorf("FetchSpecs should return nil, nil")
	}

	metrics, err := conn.FetchMetrics(context.Background())
	if err != nil || metrics != nil {
		t.Errorf("FetchMetrics should return nil, nil")
	}
}

func TestHealthCheck(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")
	_ = os.WriteFile(logFile, []byte(""), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile},
	})
	if err := conn.HealthCheck(context.Background()); err != nil {
		t.Errorf("expected healthy, got: %v", err)
	}
}

func TestHealthCheckMissingFile(t *testing.T) {
	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": "/nonexistent/access.log"},
	})
	if err := conn.HealthCheck(context.Background()); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFetchTrafficEntriesLogRotation(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "access.log")

	// Write initial log data
	bigLine := `10.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /before-rotation HTTP/1.1" 200 100 "-" "curl" 0.001`
	_ = os.WriteFile(logFile, []byte(strings.Repeat(bigLine+"\n", 100)), 0644)

	conn, _ := NewConnector(config.ConnectorConfig{
		Name:    "traffic-test",
		Options: map[string]string{"log-path": logFile, "sample-rate": "1.0"},
	})
	tc := conn.(*Connector)

	// Read to advance offset
	_, _ = tc.FetchTrafficEntries()
	if tc.lastOffset == 0 {
		t.Fatal("expected non-zero offset after first read")
	}

	// Simulate log rotation: truncate and write smaller content
	smallLine := `10.0.0.2 - - [10/Oct/2024:14:00:00 -0700] "GET /after-rotation HTTP/1.1" 200 50 "-" "curl" 0.002`
	_ = os.WriteFile(logFile, []byte(smallLine+"\n"), 0644)

	// This should detect the truncation and reset offset
	entries, err := tc.FetchTrafficEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after rotation, got %d", len(entries))
	}
	if entries[0].Path != "/after-rotation" {
		t.Errorf("expected /after-rotation, got %q", entries[0].Path)
	}
}
