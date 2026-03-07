package kong

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	apiKey     string
	httpClient *http.Client
}

func NewConnector(cfg config.ConnectorConfig) (connector.Connector, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("kong connector requires url")
	}
	return &Connector{
		name:    cfg.Name,
		baseURL: cfg.URL,
		apiKey:  cfg.Auth.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (k *Connector) Type() string { return "kong" }
func (k *Connector) Name() string { return k.name }

func (k *Connector) FetchSpecs(ctx context.Context) ([]models.APISpec, error) {
	url := fmt.Sprintf("%s/services", k.baseURL)
	body, err := k.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Path     string `json:"path"`
			Protocol string `json:"protocol"`
			Host     string `json:"host"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse services: %w", err)
	}

	var specs []models.APISpec
	for _, svc := range resp.Data {
		specs = append(specs, models.APISpec{
			APIID:    svc.ID,
			APIName:  svc.Name,
			BasePath: svc.Path,
			Metadata: map[string]string{
				"protocol": svc.Protocol,
				"host":     svc.Host,
			},
		})
	}
	return specs, nil
}

func (k *Connector) FetchMetrics(ctx context.Context) ([]models.APIMetrics, error) {
	// Kong OSS doesn't expose analytics — requires Kong Enterprise or a plugin.
	return nil, nil
}

func (k *Connector) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/status", k.baseURL)
	_, err := k.doRequest(ctx, url)
	return err
}

func (k *Connector) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if k.apiKey != "" {
		req.Header.Set("Kong-Admin-Token", k.apiKey)
	}
	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
