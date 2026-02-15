package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/game-server/node/internal/config"
	"github.com/game-server/node/internal/grpc/client"
	"github.com/game-server/node/internal/process"
	pb "github.com/game-server/node/internal/grpc/proto"
	"go.uber.org/zap"
)

// Agent represents the node agent that manages game servers
type Agent struct {
	cfg          *config.Config
	processMgr   *process.Manager
	grpcClient   *client.Client
	logger       *zap.Logger
	nodeInfo     *pb.NodeInfo
}

// NewAgent creates a new node agent
func NewAgent(
	cfg *config.Config,
	processMgr *process.Manager,
	grpcClient *client.Client,
	logger *zap.Logger,
) *Agent {
	return &Agent{
		cfg:        cfg,
		processMgr: processMgr,
		grpcClient: grpcClient,
		logger:     logger,
	}
}

// Start starts the node agent
func (a *Agent) Start(ctx context.Context) error {
	// Register with controller
	if err := a.registerWithController(ctx); err != nil {
		return fmt.Errorf("failed to register with controller: %w", err)
	}

	// Start event stream
	go a.handleEventStream(ctx)

	// Start metrics collector
	go a.collectMetrics(ctx)

	// Start health monitor
	go a.monitorHealth(ctx)

	return nil
}

// Stop stops the node agent
func (a *Agent) Stop(ctx context.Context) error {
	// Stop all running servers
	if err := a.processMgr.StopAll(ctx); err != nil {
		a.logger.Error("Failed to stop all servers", zap.Error(err))
	}

	// Close gRPC connection
	a.grpcClient.Close()

	return nil
}

// registerWithController registers the node with the controller
func (a *Agent) registerWithController(ctx context.Context) error {
	// Get node information
	nodeInfo := &pb.NodeInfo{
		NodeId:       a.cfg.NodeID,
		Hostname:     getHostname(),
		IpAddress:    getIPAddress(),
		Resources:    a.getNodeResources(),
		GameTypes:    []string{"minecraft", "valheim", "terraria", "factorio"}, // TODO: Auto-detect
		OsVersion:    runtime.GOOS + " " + runtime.GOARCH,
		AgentVersion: "1.0.0",
		Initialized:  a.processMgr.IsInitialized(),
		GameType:     a.processMgr.GetGameType(),
	}

	a.nodeInfo = nodeInfo

	// Register with controller
	response, err := a.grpcClient.RegisterNode(ctx, nodeInfo)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	a.logger.Info("Node registered with controller",
		zap.String("controller_id", response.ControllerId),
		zap.Int64("heartbeat_interval", response.HeartbeatIntervalSeconds),
		zap.Bool("initialized", nodeInfo.Initialized))

	return nil
}

// handleEventStream handles bidirectional event streaming with controller
func (a *Agent) handleEventStream(ctx context.Context) {
	stream, err := a.grpcClient.StreamEvents(ctx)
	if err != nil {
		a.logger.Error("Failed to create event stream", zap.Error(err))
		return
	}

	// Send initial online event
	if err := stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_NODE_ONLINE,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		a.logger.Error("Failed to send online event", zap.Error(err))
		return
	}

	// Handle incoming commands and outgoing events
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Send heartbeat periodically
				a.sendHeartbeat(ctx, stream)
				time.Sleep(time.Duration(a.cfg.GetHeartbeatInterval()) * time.Second)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			cmd, err := stream.Recv()
			if err != nil {
				a.logger.Error("Failed to receive command", zap.Error(err))
				return
			}
			a.handleCommand(ctx, stream, cmd)
		}
	}
}

// handleCommand handles a command from the controller
func (a *Agent) handleCommand(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	a.logger.Info("Received command",
		zap.String("command_id", cmd.CommandId),
		zap.String("command_type", cmd.Type.String()))

	switch cmd.Type {
	case pb.CommandType_COMMAND_TYPE_INITIALIZE_NODE:
		a.handleInitializeNode(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_CREATE_SERVER:
		a.handleCreateServer(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_START_SERVER:
		a.handleStartServer(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_STOP_SERVER:
		a.handleStopServer(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_RESTART_SERVER:
		a.handleRestartServer(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_UPDATE_SERVER:
		a.handleUpdateServer(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_DELETE_SERVER:
		a.handleDeleteServer(ctx, stream, cmd)
	default:
		a.logger.Warn("Unknown command type", zap.String("type", cmd.Type.String()))
	}
}

// handleInitializeNode handles node initialization
func (a *Agent) handleInitializeNode(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	initCmd := cmd.GetInitializeNode()
	if initCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid initialize command")
		return
	}

	// Check if already initialized
	if a.processMgr.IsInitialized() {
		a.logger.Warn("Node is already initialized, skipping initialization",
			zap.String("game_type", a.processMgr.GetGameType()))
		a.sendCommandResult(stream, cmd.CommandId, false, "Node is already initialized")
		return
	}

	// Send initializing event
	stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_NODE_INITIALIZING,
		Timestamp: time.Now().Unix(),
	})

	// Initialize the node (install dependencies based on game type)
	if err := a.processMgr.Initialize(ctx, initCmd.GameType); err != nil {
		if err == process.ErrAlreadyInitialized {
			a.logger.Warn("Node is already initialized", zap.String("game_type", initCmd.GameType))
			a.sendCommandResult(stream, cmd.CommandId, false, "Node is already initialized")
			return
		}
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	// Send ready event
	stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_NODE_READY,
		Timestamp: time.Now().Unix(),
	})

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// handleCreateServer handles server creation
func (a *Agent) handleCreateServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	createCmd := cmd.GetCreateServer()
	if createCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid create command")
		return
	}

	// Create server
	serverID, err := a.processMgr.CreateServer(ctx, &process.ServerConfig{
		ServerID:  createCmd.ServerId,
		GameType:  createCmd.GameType,
		Version:   createCmd.Config.Version,
		Settings:  createCmd.Config.Settings,
		MaxPlayers: int(createCmd.Config.MaxPlayers),
		Port:      0, // Auto-assign
	})

	if err != nil {
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	// Send created event
	stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_SERVER_CREATED,
		Timestamp: time.Now().Unix(),
		ServerStatus: &pb.ServerStatus{
			ServerId:  serverID,
			State:     pb.ServerState_SERVER_STATE_STOPPED,
		},
	})

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// handleStartServer handles server start
func (a *Agent) handleStartServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	startCmd := cmd.GetStartServer()
	if startCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid start command")
		return
	}

	// Start server
	if err := a.processMgr.StartServer(ctx, startCmd.ServerId); err != nil {
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	// Send started event
	stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_SERVER_STARTED,
		Timestamp: time.Now().Unix(),
		ServerStatus: &pb.ServerStatus{
			ServerId: startCmd.ServerId,
			State:    pb.ServerState_SERVER_STATE_RUNNING,
		},
	})

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// handleStopServer handles server stop
func (a *Agent) handleStopServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	stopCmd := cmd.GetStopServer()
	if stopCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid stop command")
		return
	}

	// Stop server
	if err := a.processMgr.StopServer(ctx, stopCmd.ServerId); err != nil {
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// handleRestartServer handles server restart
func (a *Agent) handleRestartServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	restartCmd := cmd.GetStopServer()
	if restartCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid restart command")
		return
	}

	// Restart server
	if err := a.processMgr.RestartServer(ctx, restartCmd.ServerId); err != nil {
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// handleUpdateServer handles server update
func (a *Agent) handleUpdateServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	a.sendCommandResult(stream, cmd.CommandId, true, "Update not implemented")
}

// handleDeleteServer handles server deletion
func (a *Agent) handleDeleteServer(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	deleteCmd := cmd.GetDeleteServer()
	if deleteCmd == nil {
		a.sendCommandResult(stream, cmd.CommandId, false, "Invalid delete command")
		return
	}

	// Delete server
	if err := a.processMgr.DeleteServer(ctx, deleteCmd.ServerId); err != nil {
		a.sendCommandResult(stream, cmd.CommandId, false, err.Error())
		return
	}

	a.sendCommandResult(stream, cmd.CommandId, true, "")
}

// sendCommandResult sends a command result to the controller
func (a *Agent) sendCommandResult(stream *client.EventStream, commandID string, success bool, message string) {
	// In a real implementation, we'd send this back via the stream
	a.logger.Info("Command result",
		zap.String("command_id", commandID),
		zap.Bool("success", success),
		zap.String("message", message))
}

// sendHeartbeat sends a heartbeat event
func (a *Agent) sendHeartbeat(ctx context.Context, stream *client.EventStream) {
	metrics := a.getCurrentMetrics()
	
	event := &pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_HEARTBEAT,
		Timestamp: time.Now().Unix(),
		Metrics: &pb.MetricsSnapshot{
			NodeId:          a.cfg.NodeID,
			CpuUsagePercent: float32(metrics.CPUUsage),
			MemoryUsagePercent: float32(metrics.MemoryUsage),
			Timestamp:       time.Now().Unix(),
		},
	}

	if err := stream.Send(event); err != nil {
		a.logger.Error("Failed to send heartbeat", zap.Error(err))
	}
}

// collectMetrics periodically collects and sends metrics
func (a *Agent) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := a.getCurrentMetrics()
			a.processMgr.UpdateMetrics(metrics)
		}
	}
}

// monitorHealth monitors node health
func (a *Agent) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check process health
			a.processMgr.CheckHealth()
		}
	}
}

// getNodeResources returns the node resources
func (a *Agent) getNodeResources() *pb.NodeResources {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &pb.NodeResources{
		TotalCpuCores:       int32(runtime.NumCPU()),
		TotalMemoryMb:       int64(m.Sys) / (1024 * 1024),
		TotalStorageMb:     1024 * 1024, // TODO: Get actual storage
		AvailableCpuCores:  int32(runtime.NumCPU()),
		AvailableMemoryMb:  int64(m.Sys) / (1024 * 1024),
		AvailableStorageMb: 1024 * 1024,
	}
}

// getCurrentMetrics returns current system metrics
func (a *Agent) getCurrentMetrics() *process.SystemMetrics {
	return a.processMgr.GetSystemMetrics()
}

// Helper functions

func getHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}

func getIPAddress() string {
	// TODO: Get actual IP address
	return "127.0.0.1"
}
