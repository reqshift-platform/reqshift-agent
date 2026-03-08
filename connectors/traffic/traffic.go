package traffic

import (
	"bufio"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

var _ connector.Connector = (*Connector)(nil)

// Nginx combined log format regex.
var nginxCombinedRegex = regexp.MustCompile(
	`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) \d+.*?(\d+\.\d+)?$`)

// TrafficEntry represents a parsed access log entry.
type TrafficEntry struct {
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Status    int     `json:"status"`
	LatencyMs float64 `json:"latencyMs"`
	SourceIP  string  `json:"sourceIp"`
	Timestamp int64   `json:"timestamp"`
}

type Connector struct {
	name       string
	logPath    string
	sampleRate float64
	lastOffset int64
}

func NewConnector(cfg config.ConnectorConfig) (connector.Connector, error) {
	logPath := cfg.Options["log-path"]
	if logPath == "" {
		return nil, fmt.Errorf("traffic-logs connector requires options.log-path")
	}

	sampleRate := 0.1
	if rateStr, ok := cfg.Options["sample-rate"]; ok {
		if parsed, err := strconv.ParseFloat(rateStr, 64); err == nil && parsed > 0 && parsed <= 1.0 {
			sampleRate = parsed
		}
	}

	return &Connector{
		name:       cfg.Name,
		logPath:    logPath,
		sampleRate: sampleRate,
	}, nil
}

func (t *Connector) Type() string { return "traffic-logs" }
func (t *Connector) Name() string { return t.name }

// FetchSpecs returns nil — traffic connector doesn't discover API specifications.
func (t *Connector) FetchSpecs(_ context.Context) ([]models.APISpec, error) {
	return nil, nil
}

// FetchMetrics returns nil — traffic data is not aggregated into APIMetrics.
// Use FetchTrafficEntries for raw log data.
func (t *Connector) FetchMetrics(_ context.Context) ([]models.APIMetrics, error) {
	return nil, nil
}

// FetchTrafficEntries reads new log lines since last offset.
func (t *Connector) FetchTrafficEntries() ([]TrafficEntry, error) {
	file, err := os.Open(t.logPath)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	if t.lastOffset > 0 {
		// Detect log rotation: if file is smaller than last offset, reset.
		if info, err := file.Stat(); err == nil && info.Size() < t.lastOffset {
			t.lastOffset = 0
		} else if _, err := file.Seek(t.lastOffset, 0); err != nil {
			t.lastOffset = 0
			_, _ = file.Seek(0, 0)
		}
	}

	var entries []TrafficEntry
	var bytesRead int64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		bytesRead += int64(len(scanner.Bytes())) + 1 // +1 for newline

		if rand.Float64() > t.sampleRate {
			continue
		}

		entry, err := t.parseLine(line)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	t.lastOffset += bytesRead

	return entries, nil
}

func (t *Connector) HealthCheck(_ context.Context) error {
	_, err := os.Stat(t.logPath)
	return err
}

func (t *Connector) parseLine(line string) (TrafficEntry, error) {
	matches := nginxCombinedRegex.FindStringSubmatch(line)
	if len(matches) < 6 {
		return TrafficEntry{}, fmt.Errorf("no match")
	}

	status, _ := strconv.Atoi(matches[5])

	var latency float64
	if len(matches) > 6 && matches[6] != "" {
		latency, _ = strconv.ParseFloat(matches[6], 64)
		latency *= 1000
	}

	ts, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2])
	if err != nil {
		ts = time.Now()
	}

	path := matches[4]
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	return TrafficEntry{
		Method:    matches[3],
		Path:      path,
		Status:    status,
		LatencyMs: latency,
		SourceIP:  anonymizeIP(matches[1]),
		Timestamp: ts.UnixMilli(),
	}, nil
}

func anonymizeIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		parts[3] = "0"
		return strings.Join(parts, ".")
	}
	return ip
}
