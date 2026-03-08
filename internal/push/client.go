package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// Client pushes data to the Reqshift Ingestion API.
// All communication is outbound HTTPS on port 443.
// No inbound ports needed — firewall-friendly.
type Client struct {
	endpoint   string
	apiKey     string
	agentID    string
	version    string
	httpClient *http.Client
}

func NewClient(endpoint, apiKey, agentID, version string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		agentID:  agentID,
		version:  version,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// PushSync sends a full sync payload (specs + metrics + health).
func (c *Client) PushSync(ctx context.Context, payload *models.SyncPayload) (*models.SyncResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := c.doPost(ctx, "/ingest/sync", body)
	if err != nil {
		return nil, err
	}

	var result models.SyncResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// PushHeartbeat sends a lightweight health check.
func (c *Client) PushHeartbeat(ctx context.Context, health *models.AgentHealth) error {
	body, err := json.Marshal(health)
	if err != nil {
		return fmt.Errorf("marshal health: %w", err)
	}

	_, err = c.doPost(ctx, "/ingest/heartbeat", body)
	return err
}

const (
	maxRetries  = 3
	baseBackoff = 1 * time.Second
)

// httpError is a structured HTTP error carrying the status code.
type httpError struct {
	StatusCode int
	Body       string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

func (c *Client) doPost(ctx context.Context, path string, body []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := baseBackoff * (1 << (attempt - 1)) // 1s, 2s, 4s
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}

		respBody, err := c.doPostOnce(ctx, path, body)
		if err == nil {
			return respBody, nil
		}
		lastErr = err

		// Only retry on network errors or 5xx — not 4xx.
		var he *httpError
		if errors.As(err, &he) && he.StatusCode >= 400 && he.StatusCode < 500 {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) doPostOnce(ctx context.Context, path string, body []byte) ([]byte, error) {
	url := c.endpoint + path

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", c.apiKey)
	req.Header.Set("X-Agent-Id", c.agentID)
	req.Header.Set("User-Agent", "reqshift-agent/"+c.version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return respBody, nil
}
