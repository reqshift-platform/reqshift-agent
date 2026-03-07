# ════════════════════════════════════════
# Reqshift Agent — Multi-stage Dockerfile
# Final image: ~15MB (scratch + static binary)
# ════════════════════════════════════════

# ── Build stage ──
FROM golang:1.24-alpine AS builder

ARG VERSION=dev

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /reqshift-agent \
    ./cmd/reqshift-agent

# ── Runtime stage ──
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /reqshift-agent /reqshift-agent

ENTRYPOINT ["/reqshift-agent"]
CMD ["--config", "/etc/reqshift/agent.yaml"]
