# Reqshift Agent

[![CI](https://github.com/reqshift-platform/reqshift-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/reqshift-platform/reqshift-agent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Lightweight agent that discovers API specifications and runtime metrics from your infrastructure and syncs them to [Reqshift Cloud](https://reqshift.io).

## Architecture

```
                          +-----------------+
                          | Reqshift Cloud  |
                          |  (Ingestion API)|
                          +--------^--------+
                                   |
                            HTTPS (443)
                            outbound only
                                   |
                          +--------+--------+
                          |  Reqshift Agent |
                          |  (scheduler)    |
                          +--------+--------+
                                   |
              +--------------------+--------------------+
              |                    |                     |
     +--------v-----+   +--------v------+   +----------v---------+
     |   Gravitee   |   |     Kong      |   |   OpenAPI (files)  |
     |   Connector  |   |   Connector   |   |     Connector      |
     +--------------+   +---------------+   +--------------------+
              |
     +--------v---------+
     | Traffic Logs      |
     | (Nginx parser)    |
     +-------------------+
```

**Key design choices:**
- **Outbound only** — no inbound ports needed, firewall-friendly
- **Plugin architecture** — each connector implements the same interface
- **Concurrent syncs** — one goroutine per connector, independent intervals
- **Minimal dependencies** — only `gopkg.in/yaml.v3`, everything else is stdlib

## Supported Connectors

| Connector | Discovers | Metrics | Auth |
|-----------|-----------|---------|------|
| **Gravitee APIM** | API specs + definitions | Hits, latency P50/P95/P99 | Bearer token |
| **Kong Gateway** | Services (name, path, host) | N/A (requires Enterprise) | Admin token (optional) |
| **OpenAPI (files)** | Local `.json`/`.yaml`/`.yml` files | N/A | N/A |
| **Traffic Logs** | N/A | Nginx access log parsing with sampling | N/A |

## Installation

### Binary

Download from the [latest release](https://github.com/reqshift-platform/reqshift-agent/releases):

```bash
# Linux AMD64
curl -LO https://github.com/reqshift-platform/reqshift-agent/releases/latest/download/reqshift-agent-linux-amd64
chmod +x reqshift-agent-linux-amd64
sudo mv reqshift-agent-linux-amd64 /usr/local/bin/reqshift-agent
```

### Docker

```bash
docker run -v /path/to/agent.yaml:/etc/reqshift/agent.yaml \
  ghcr.io/reqshift-platform/reqshift-agent:latest
```

### From Source

```bash
git clone https://github.com/reqshift-platform/reqshift-agent.git
cd reqshift-agent
go build -o reqshift-agent ./cmd/reqshift-agent
```

## Configuration

Create an `agent.yaml` file:

```yaml
# Agent identity
agent:
  id: agent-paris-01           # Unique agent ID
  name: Paris Production        # Human-readable name

# Reqshift Cloud connection
cloud:
  endpoint: https://api.reqshift.io   # Ingestion API URL
  api-key: ${REQSHIFT_API_KEY}        # API key (supports env vars)

# Connectors to activate
connectors:
  # Gravitee APIM
  - type: gravitee
    name: gravitee-prod
    url: http://gravitee-management:8083
    auth:
      type: bearer
      token: ${GRAVITEE_TOKEN}
    sync-interval: 5m                  # Default: 5m
    options:
      environment: DEFAULT             # Gravitee environment ID

  # Kong Gateway
  - type: kong
    name: kong-prod
    url: http://kong-admin:8001
    auth:
      token: ${KONG_ADMIN_TOKEN}       # Optional
    sync-interval: 5m

  # Local OpenAPI files
  - type: openapi
    name: local-specs
    options:
      watch-dir: /opt/api-specs        # Or use `url:` field
    sync-interval: 10m

  # Nginx access logs
  - type: traffic-logs
    name: nginx-traffic
    options:
      log-path: /var/log/nginx/access.log
      sample-rate: "0.1"               # 10% sampling (default)
    sync-interval: 1m
```

### Configuration Reference

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `agent.id` | Yes | — | Unique agent identifier |
| `agent.name` | No | — | Human-readable name |
| `cloud.endpoint` | Yes | — | Reqshift Ingestion API URL |
| `cloud.api-key` | Yes | — | API key for authentication |
| `connectors[].type` | Yes | — | `gravitee`, `kong`, `openapi`, or `traffic-logs` |
| `connectors[].name` | No | — | Connector display name |
| `connectors[].url` | Varies | — | Connector endpoint URL |
| `connectors[].sync-interval` | No | `5m` | Sync frequency |
| `connectors[].auth.type` | No | — | `bearer`, `basic`, or `apikey` |
| `connectors[].auth.token` | No | — | Auth token |

Environment variables in values (`${VAR}`) are automatically expanded.

## Deployment Examples

### Docker Compose

```yaml
services:
  reqshift-agent:
    image: ghcr.io/reqshift-platform/reqshift-agent:v1.0.0
    restart: unless-stopped
    volumes:
      - ./agent.yaml:/etc/reqshift/agent.yaml:ro
      - /var/log/nginx:/var/log/nginx:ro      # For traffic-logs connector
    environment:
      - REQSHIFT_API_KEY=${REQSHIFT_API_KEY}
      - GRAVITEE_TOKEN=${GRAVITEE_TOKEN}
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reqshift-agent
spec:
  replicas: 1
  selector:
    matchLabels:
      app: reqshift-agent
  template:
    metadata:
      labels:
        app: reqshift-agent
    spec:
      containers:
        - name: agent
          image: ghcr.io/reqshift-platform/reqshift-agent:v1.0.0
          volumeMounts:
            - name: config
              mountPath: /etc/reqshift
          env:
            - name: REQSHIFT_API_KEY
              valueFrom:
                secretKeyRef:
                  name: reqshift-secrets
                  key: api-key
      volumes:
        - name: config
          configMap:
            name: reqshift-agent-config
```

### Systemd

```ini
[Unit]
Description=Reqshift Agent
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/reqshift-agent -config /etc/reqshift/agent.yaml
Restart=always
RestartSec=10
EnvironmentFile=/etc/reqshift/env

[Install]
WantedBy=multi-user.target
```

## Development

### Prerequisites

- Go 1.24+ or Docker

### Run Tests

```bash
# Via Docker (no local Go required)
docker run --rm -v "$(pwd):/src" -w /src golang:1.24-alpine \
  go test -race -v ./...

# With coverage
docker run --rm -v "$(pwd):/src" -w /src golang:1.24-alpine \
  sh -c "go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out"

# Local (requires Go 1.24+)
go test -race -v ./...
```

### Lint

```bash
docker run --rm -v "$(pwd):/src" -w /src golangci/golangci-lint:latest \
  golangci-lint run ./...
```

### Build

```bash
# Binary
go build -ldflags="-s -w -X main.version=v1.0.0" -o reqshift-agent ./cmd/reqshift-agent

# Docker image
docker build --build-arg VERSION=v1.0.0 -t reqshift-agent .

# Verify
./reqshift-agent --version
```

## License

[Apache License 2.0](LICENSE)
