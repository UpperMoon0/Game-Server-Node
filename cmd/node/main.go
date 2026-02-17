package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/game-server/node/internal/agent"
	"github.com/game-server/node/internal/config"
	"github.com/game-server/node/internal/files"
	"github.com/game-server/node/internal/grpc/client"
	_ "github.com/game-server/node/internal/grpc/codec" // Register JSON codec
	"github.com/game-server/node/internal/process"
	"github.com/game-server/node/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	configPath := "config.yaml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Initialize logger
	log, err := logger.NewFromConfig(cfg.LogLevel, cfg.LogFormat, cfg.LogFilePath)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer log.Sync()

	log.Info("Starting Game Server Node Agent",
		zap.String("node_id", cfg.NodeID),
		zap.String("controller_address", cfg.ControllerAddress))

	// Initialize process manager
	processMgr, err := process.NewManager(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize process manager", zap.Error(err))
	}

	// Initialize file manager
	fileMgr := files.NewManager(cfg.DataPath)
	log.Info("File manager initialized", zap.String("base_path", cfg.DataPath))

	// Initialize gRPC client
	grpcClient, err := client.NewClient(cfg.ControllerAddress, log)
	if err != nil {
		log.Fatal("Failed to create gRPC client", zap.Error(err))
	}

	// Initialize agent
	nodeAgent := agent.NewAgent(cfg, processMgr, fileMgr, grpcClient, log)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start agent
	if err := nodeAgent.Start(ctx); err != nil {
		log.Fatal("Failed to start agent", zap.Error(err))
	}

	log.Info("Node agent is running",
		zap.String("node_id", cfg.NodeID),
		zap.String("grpc_address", cfg.GRPCAddress))

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down node agent...")

	// Cancel context
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop agent
	if err := nodeAgent.Stop(shutdownCtx); err != nil {
		log.Error("Failed to stop agent gracefully", zap.Error(err))
	}

	log.Info("Node agent stopped")
}
