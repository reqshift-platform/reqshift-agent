package gravitee

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

var _ connector.Connector = (*Connector)(nil)

type Connector struct {
	name       string
	baseURL    string
	token      string
	httpClient *http.Client
	envID      string
}

func NewConnector(cfg config.ConnectorConfig) (connector.Connector, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("gravitee connector requires url")
	}

	envID := cfg.Options["environment"]
	if envID == "" {
		envID = "DEFAULT"
	}

	return &Connector{
		name:    cfg.Name,
		baseURL: cfg.URL,
		token:   cfg.Auth.Token,
		envID:   envID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (g *Connector) Type() string { return "gravitee" }
func (g *Connector) Name() string { return g.name }

func (g *Connector) FetchSpecs(ctx context.Context) ([]models.APISpec, error) {
	url := fmt.Sprintf("%s/management/v2/environments/%s/apis?size=100", g.baseURL, g.envID)
	body, err := g.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list APIs: %w", err)
	}

	var listResp struct {
		Data []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			APIVersion  string    `json:"apiVersion"`
			UpdatedAt   time.Time `json:"updatedAt"`
			ContextPath string    `json:"contextPath"`
			Tags        []string  `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("parse API list: %w", err)
	}

	var specs []models.APISpec
	for _, api := range listResp.Data {
		specURL := fmt.Sprintf("%s/management/v2/environments/%s/apis/%s/definition",
			g.baseURL, g.envID, api.ID)
		specBody, err := g.doRequest(ctx, specURL)
		if err != nil {
			slog.Warn("skipping API spec", "apiId", api.ID, "error", err)
			continue
		}

		specs = append(specs, models.APISpec{
			APIID:        api.ID,
			APIName:      api.Name,
			Version:      api.APIVersion,
			BasePath:     api.ContextPath,
			SpecFormat:   models.DetectSpecFormat(string(specBody)),
			SpecContent:  string(specBody),
			Tags:         api.Tags,
			LastModified: api.UpdatedAt,
		})
	}

	return specs, nil
}

func (g *Connector) FetchMetrics(ctx context.Context) ([]models.APIMetrics, error) {
	url := fmt.Sprintf("%s/management/v2/environments/%s/analytics?type=group_by&field=api&interval=60000",
		g.baseURL, g.envID)
	body, err := g.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch analytics: %w", err)
	}

	var resp struct {
		Values map[string]struct {
			Hits    int64   `json:"hits"`
			AvgTime float64 `json:"avg"`
			P95Time float64 `json:"p95"`
			P99Time float64 `json:"p99"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse analytics: %w", err)
	}

	now := time.Now()
	var metrics []models.APIMetrics
	for apiID, data := range resp.Values {
		metrics = append(metrics, models.APIMetrics{
			APIID:        apiID,
			RequestCount: data.Hits,
			LatencyP50Ms: data.AvgTime,
			LatencyP95Ms: data.P95Time,
			LatencyP99Ms: data.P99Time,
			PeriodStart:  now.Add(-5 * time.Minute),
			PeriodEnd:    now,
		})
	}

	return metrics, nil
}

func (g *Connector) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/management/v2/environments/%s", g.baseURL, g.envID)
	_, err := g.doRequest(ctx, url)
	return err
}

func (g *Connector) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
