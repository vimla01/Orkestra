package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/orkestra/internal/api"
	"github.com/orkestra/internal/config"
	"github.com/orkestra/internal/health"
	"github.com/orkestra/internal/k8s"
	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
)

func main() {
	// Check for subcommands before parsing flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "cluster":
			handleClusterCommand(os.Args[2:])
			return
		case "deploy":
			handleDeployCommand(os.Args[2:])
			return
		case "deployment":
			handleDeploymentCommand(os.Args[2:])
			return
		case "serve":
			// Drop the subcommand so the server flags below still parse.
			os.Args = append(os.Args[:1], os.Args[2:]...)
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	// Server mode
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Configure logger
	logger := logrus.New()
	level, err := logrus.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Create Kubernetes client factory
	clientFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
		return k8s.NewClientFromKubeconfig(kubeconfigPath)
	}

	// Create registry
	reg := registry.NewRegistry(clientFactory)

	// Create health aggregator
	healthInterval := time.Duration(cfg.Health.PollIntervalSeconds) * time.Second
	aggregator := health.NewAggregator(reg, clientFactory, healthInterval, logger)

	// Create propagation engine
	engine := propagation.NewEngine(reg, clientFactory, logger)

	// Create API server
	server := api.NewServer(cfg.Server.Port, reg, aggregator, engine, logger)

	// Print startup banner
	printBanner()
	logger.Infof("Orkestra control plane starting on port %d", cfg.Server.Port)

	// Create context that listens for SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start health aggregator
	aggregator.Start(ctx)

	// Start failover controller, reconciling after each health poll interval
	if cfg.Failover.Enabled {
		gracePeriod := time.Duration(cfg.Failover.GracePeriodSeconds) * time.Second
		engine.StartFailover(ctx, healthInterval, gracePeriod)
	}

	// Start API server in a goroutine
	go func() {
		if err := server.Start(); err != nil {
			logger.Errorf("API server error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Shutdown signal received, initiating graceful shutdown...")

	// Create a deadline for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("API server shutdown error: %v", err)
	}

	logger.Info("Orkestra control plane shut down gracefully")
}

func printBanner() {
	banner := `
   ____       _              _             
  / __ \_____| | _____  ____| |_ _ __ __ _ 
 / / _` + "`" + ` / ___| |/ / _ \/ ___| __| '__/ _` + "`" + ` |
| | (_| | |  |   <  __/\__ \ |_| | | (_| |
 \ \__,_|_|  |_|\_\___||___/\__|_|  \__,_|
  \____/                                   
`
	fmt.Println(banner)
}
