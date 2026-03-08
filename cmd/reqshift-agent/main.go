package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/reqshift-platform/reqshift-agent/connectors/gravitee"
	"github.com/reqshift-platform/reqshift-agent/connectors/kong"
	"github.com/reqshift-platform/reqshift-agent/connectors/openapi"
	"github.com/reqshift-platform/reqshift-agent/connectors/traffic"
	"github.com/reqshift-platform/reqshift-agent/internal/config"
	"github.com/reqshift-platform/reqshift-agent/internal/connector"
	"github.com/reqshift-platform/reqshift-agent/internal/health"
	"github.com/reqshift-platform/reqshift-agent/internal/push"
	"github.com/reqshift-platform/reqshift-agent/internal/scheduler"
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

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Register connector factories
	registry := connector.NewRegistry()
	registry.RegisterFactory(models.ConnectorGravitee, gravitee.NewConnector)
	registry.RegisterFactory(models.ConnectorKong, kong.NewConnector)
	registry.RegisterFactory(models.ConnectorOpenAPI, openapi.NewConnector)
	registry.RegisterFactory(models.ConnectorTrafficLogs, traffic.NewConnector)

	// Create connectors from config
	for _, connCfg := range cfg.Connectors {
		conn, err := registry.Create(connCfg)
		if err != nil {
			slog.Error("failed to create connector",
				"name", connCfg.Name, "type", connCfg.Type, "error", err)
			os.Exit(1)
		}
		registry.Register(conn, connCfg.SyncInterval)
	}

	pusher := push.NewClient(cfg.Cloud.Endpoint, cfg.Cloud.APIKey, cfg.Agent.ID, version)
	healthMon := health.NewMonitor(cfg.Agent.ID)

	sched := scheduler.New(registry, pusher, healthMon, cfg, version)
	sched.Start()

	slog.Info("agent started",
		"version", version,
		"agent", cfg.Agent.ID,
		"connectors", len(cfg.Connectors))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down")
	sched.Stop()
	healthMon.Stop()
}
