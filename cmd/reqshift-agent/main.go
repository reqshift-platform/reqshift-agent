package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/reqshift-platform/reqshift-agent/connectors/gravitee"
	"github.com/reqshift-platform/reqshift-agent/connectors/kong"
	"github.com/reqshift-platform/reqshift-agent/connectors/openapi"
	"github.com/reqshift-platform/reqshift-agent/connectors/traffic"
	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/internal/health"
	"github.com/reqshift-platform/reqshift-agent/internal/metrics"
	"github.com/reqshift-platform/reqshift-agent/internal/push"
	"github.com/reqshift-platform/reqshift-agent/internal/scheduler"
	"github.com/reqshift-platform/reqshift-agent/internal/server"
	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// Set at build time via -ldflags.
var (
	version   = "dev"
	buildDate = "unknown"
)

func main() {
	configPath := flag.String("config", "/etc/reqshift/agent.yaml", "Path to config file")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("reqshift-agent %s (built %s)\n", version, buildDate)
		os.Exit(0)
	}

	// Dynamic log level for SIGHUP reload.
	logLevel := &slog.LevelVar{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setLogLevel(logLevel, cfg.Logging.Level)

	// Initialize Prometheus metrics.
	metrics.Init()

	// Register connector factories.
	registry := newRegistry(cfg)

	pusher := push.NewClient(cfg.Cloud.Endpoint, cfg.Cloud.APIKey, cfg.Agent.ID, version)
	healthMon := health.NewMonitor(cfg.Agent.ID)

	// Start HTTP server (health probes + metrics).
	srv := server.New(cfg.Server.Listen, healthMon, version)
	srv.Mux().Handle("GET /metrics", promhttp.Handler())
	if err := srv.Start(); err != nil {
		slog.Error("failed to start http server", "error", err)
		os.Exit(1)
	}

	sched := scheduler.New(registry, pusher, healthMon, cfg, version)
	sched.Start()

	slog.Info("agent started",
		"version", version,
		"agent", cfg.Agent.ID,
		"connectors", len(cfg.Connectors),
		"deltaSync", cfg.Agent.DeltaSync,
		"listen", cfg.Server.Listen)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for sig := range sigCh {
		if sig == syscall.SIGHUP {
			slog.Info("SIGHUP received, reloading config")
			newCfg, err := config.Load(*configPath)
			if err != nil {
				slog.Error("config reload failed", "error", err)
				continue
			}

			sched.Stop()

			registry = newRegistry(newCfg)
			pusher = push.NewClient(newCfg.Cloud.Endpoint, newCfg.Cloud.APIKey, newCfg.Agent.ID, version)
			sched = scheduler.New(registry, pusher, healthMon, newCfg, version)
			sched.Start()

			setLogLevel(logLevel, newCfg.Logging.Level)
			cfg = newCfg
			slog.Info("config reloaded",
				"agent", cfg.Agent.ID,
				"connectors", len(cfg.Connectors))
		} else {
			break
		}
	}

	slog.Info("shutting down")
	sched.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Stop(shutdownCtx)

	healthMon.Stop()
}

func newRegistry(cfg *config.Config) *connector.Registry {
	registry := connector.NewRegistry()
	registry.RegisterFactory(models.ConnectorGravitee, gravitee.NewConnector)
	registry.RegisterFactory(models.ConnectorKong, kong.NewConnector)
	registry.RegisterFactory(models.ConnectorOpenAPI, openapi.NewConnector)
	registry.RegisterFactory(models.ConnectorTrafficLogs, traffic.NewConnector)

	for _, connCfg := range cfg.Connectors {
		conn, err := registry.Create(connCfg)
		if err != nil {
			slog.Error("failed to create connector",
				"name", connCfg.Name, "type", connCfg.Type, "error", err)
			os.Exit(1)
		}
		registry.Register(conn, connCfg.SyncInterval)
	}
	return registry
}

func setLogLevel(lv *slog.LevelVar, level string) {
	switch strings.ToLower(level) {
	case "debug":
		lv.Set(slog.LevelDebug)
	case "warn":
		lv.Set(slog.LevelWarn)
	case "error":
		lv.Set(slog.LevelError)
	default:
		lv.Set(slog.LevelInfo)
	}
}
