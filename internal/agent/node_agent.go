package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/game-server/node/internal/config"
	"github.com/game-server/node/internal/files"
	"github.com/game-server/node/internal/grpc/client"
	"github.com/game-server/node/internal/process"
	pb "github.com/nstut/game-server-proto/gen"
	"go.uber.org/zap"
)

// Agent represents the node agent that manages game servers
type Agent struct {
	cfg        *config.Config
	processMgr *process.Manager
	fileMgr    *files.Manager
	grpcClient *client.Client
	logger     *zap.Logger
	nodeInfo   *pb.RegisterNodeRequest
}

// NewAgent creates a new node agent
func NewAgent(
	cfg *config.Config,
	processMgr *process.Manager,
	fileMgr *files.Manager,
	grpcClient *client.Client,
	logger *zap.Logger,
) *Agent {
	return &Agent{
		cfg:        cfg,
		processMgr: processMgr,
		fileMgr:    fileMgr,
		grpcClient: grpcClient,
		logger:     logger,
	}
}

// Start starts the node agent
func (a *Agent) Start(ctx context.Context) error {
	// Start event stream with automatic reconnection
	go a.handleEventStreamWithReconnect(ctx)

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
	nodeInfo := &pb.RegisterNodeRequest{
		NodeId:         a.cfg.NodeID,
		Hostname:       getHostname(),
		IpAddress:      getIPAddress(),
		Resources:      a.getNodeResources(),
		SupportedGames: []string{"minecraft", "valheim", "terraria", "factorio"}, // TODO: Auto-detect
		OsVersion:      runtime.GOOS + " " + runtime.GOARCH,
		AgentVersion:   "1.0.0",
		Initialized:    a.processMgr.IsInitialized(),
		GameType:       a.processMgr.GetGameType(),
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
func (a *Agent) handleEventStream(ctx context.Context) error {
	stream, err := a.grpcClient.StreamEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to create event stream: %w", err)
	}

	// Send initial online event
	if err := stream.Send(&pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_NODE_ONLINE,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("failed to send online event: %w", err)
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
			return ctx.Err()
		default:
			cmd, err := stream.Recv()
			if err != nil {
				return fmt.Errorf("failed to receive command: %w", err)
			}
			a.handleCommand(ctx, stream, cmd)
		}
	}
}

// handleEventStreamWithReconnect handles event streaming with automatic reconnection
func (a *Agent) handleEventStreamWithReconnect(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Try to reconnect gRPC client if needed
		if a.grpcClient.NeedsReconnect() {
			a.logger.Info("Reconnecting to controller...")
			if err := a.grpcClient.Reconnect(); err != nil {
				a.logger.Error("Failed to reconnect, retrying...",
					zap.Error(err),
					zap.Duration("backoff", backoff))
				time.Sleep(backoff)
				backoff = a.calculateBackoff(backoff, maxBackoff)
				continue
			}
		}

		// Try to register with controller
		if err := a.registerWithController(ctx); err != nil {
			a.logger.Error("Failed to register with controller, retrying...",
				zap.Error(err),
				zap.Duration("backoff", backoff))
			time.Sleep(backoff)
			backoff = a.calculateBackoff(backoff, maxBackoff)
			continue
		}

		// Reset backoff on successful connection
		backoff = 1 * time.Second

		// Handle event stream
		err := a.handleEventStream(ctx)
		if err != nil {
			a.logger.Error("Event stream error, reconnecting...",
				zap.Error(err),
				zap.Duration("backoff", backoff))
			time.Sleep(backoff)
			backoff = a.calculateBackoff(backoff, maxBackoff)
		}
	}
}

// calculateBackoff calculates the next backoff duration with exponential backoff
func (a *Agent) calculateBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
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
	case pb.CommandType_COMMAND_TYPE_DELETE_SERVER:
		a.handleDeleteServer(ctx, stream, cmd)
	// File operations
	case pb.CommandType_COMMAND_TYPE_FILE_LIST:
		a.handleFileList(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_CREATE:
		a.handleFileCreate(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_DELETE:
		a.handleFileDelete(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_RENAME:
		a.handleFileRename(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_MOVE:
		a.handleFileMove(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_COPY:
		a.handleFileCopy(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_WRITE:
		a.handleFileWrite(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_READ:
		a.handleFileRead(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_ZIP:
		a.handleFileZip(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_UNZIP:
		a.handleFileUnzip(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_EXISTS:
		a.handleFileExists(ctx, stream, cmd)
	case pb.CommandType_COMMAND_TYPE_FILE_MKDIR:
		a.handleFileMkdir(ctx, stream, cmd)
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
		Payload: &pb.NodeEvent_ServerStatus{
			ServerStatus: &pb.ServerStatus{
				ServerId:  serverID,
				State:     pb.ServerState_SERVER_STATE_STOPPED,
			},
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
		Payload: &pb.NodeEvent_ServerStatus{
			ServerStatus: &pb.ServerStatus{
				ServerId: startCmd.ServerId,
				State:    pb.ServerState_SERVER_STATE_RUNNING,
			},
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

// File operation handlers

// handleFileList handles file list command
func (a *Agent) handleFileList(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	listCmd := cmd.GetFileList()
	if listCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid list command", nil)
		return
	}

	files, err := a.fileMgr.ListFiles(listCmd.Path, listCmd.Recursive)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	// Convert to proto FileInfo
	protoFiles := make([]*pb.FileInfo, len(files))
	for i, f := range files {
		protoFiles[i] = &pb.FileInfo{
			Name:         f.Name,
			Path:         f.Path,
			IsDirectory:  f.IsDirectory,
			Size:         f.Size,
			ModifiedTime: f.ModifiedTime,
			CreatedTime:  f.CreatedTime,
			Permissions:  f.Permissions,
		}
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", &pb.FileListResult{
		Files:       protoFiles,
		CurrentPath: listCmd.Path,
	})
}

// handleFileCreate handles file create command
func (a *Agent) handleFileCreate(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	createCmd := cmd.GetFileCreate()
	if createCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid create command", nil)
		return
	}

	err := a.fileMgr.CreateFile(createCmd.Path, createCmd.IsDirectory)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileDelete handles file delete command
func (a *Agent) handleFileDelete(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	deleteCmd := cmd.GetFileDelete()
	if deleteCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid delete command", nil)
		return
	}

	err := a.fileMgr.DeleteFile(deleteCmd.Path, deleteCmd.Recursive)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileRename handles file rename command
func (a *Agent) handleFileRename(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	renameCmd := cmd.GetFileRename()
	if renameCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid rename command", nil)
		return
	}

	err := a.fileMgr.RenameFile(renameCmd.OldPath, renameCmd.NewPath)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileMove handles file move command
func (a *Agent) handleFileMove(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	moveCmd := cmd.GetFileMove()
	if moveCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid move command", nil)
		return
	}

	err := a.fileMgr.MoveFile(moveCmd.SourcePath, moveCmd.DestPath)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileCopy handles file copy command
func (a *Agent) handleFileCopy(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	copyCmd := cmd.GetFileCopy()
	if copyCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid copy command", nil)
		return
	}

	err := a.fileMgr.CopyFile(copyCmd.SourcePath, copyCmd.DestPath, copyCmd.Recursive)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileWrite handles file write command
func (a *Agent) handleFileWrite(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	writeCmd := cmd.GetFileWrite()
	if writeCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid write command", nil)
		return
	}

	err := a.fileMgr.WriteFile(writeCmd.Path, writeCmd.Content, writeCmd.Append)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileRead handles file read command
func (a *Agent) handleFileRead(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	readCmd := cmd.GetFileRead()
	if readCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid read command", nil)
		return
	}

	content, totalSize, err := a.fileMgr.ReadFile(readCmd.Path, readCmd.Offset, readCmd.Length)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", &pb.FileReadResult{
		Content:   content,
		TotalSize: totalSize,
	})
}

// handleFileZip handles file zip command
func (a *Agent) handleFileZip(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	zipCmd := cmd.GetFileZip()
	if zipCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid zip command", nil)
		return
	}

	err := a.fileMgr.ZipFiles(zipCmd.SourcePath, zipCmd.DestPath, zipCmd.Recursive)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileUnzip handles file unzip command
func (a *Agent) handleFileUnzip(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	unzipCmd := cmd.GetFileUnzip()
	if unzipCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid unzip command", nil)
		return
	}

	err := a.fileMgr.UnzipFiles(unzipCmd.SourcePath, unzipCmd.DestPath)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// handleFileExists handles file exists command
func (a *Agent) handleFileExists(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	existsCmd := cmd.GetFileExists()
	if existsCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid exists command", nil)
		return
	}

	exists, isDir, err := a.fileMgr.FileExists(existsCmd.Path)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", &pb.FileExistsResult{
		Exists:      exists,
		IsDirectory: isDir,
	})
}

// handleFileMkdir handles mkdir command
func (a *Agent) handleFileMkdir(ctx context.Context, stream *client.EventStream, cmd *pb.ControllerCommand) {
	mkdirCmd := cmd.GetFileMkdir()
	if mkdirCmd == nil {
		a.sendFileResult(stream, cmd.CommandId, false, "Invalid mkdir command", nil)
		return
	}

	err := a.fileMgr.Mkdir(mkdirCmd.Path, mkdirCmd.Parents)
	if err != nil {
		a.sendFileResult(stream, cmd.CommandId, false, err.Error(), nil)
		return
	}

	a.sendFileResult(stream, cmd.CommandId, true, "", nil)
}

// sendFileResult sends a file operation result to the controller
func (a *Agent) sendFileResult(stream *client.EventStream, commandID string, success bool, errMsg string, result interface{}) {
	fileResult := &pb.FileOperationResult{
		CommandId: commandID,
		Success:   success,
		Error:     errMsg,
	}

	if result != nil {
		switch r := result.(type) {
		case *pb.FileListResult:
			fileResult.Result = &pb.FileOperationResult_ListResult{ListResult: r}
		case *pb.FileReadResult:
			fileResult.Result = &pb.FileOperationResult_ReadResult{ReadResult: r}
		case *pb.FileExistsResult:
			fileResult.Result = &pb.FileOperationResult_ExistsResult{ExistsResult: r}
		}
	}

	event := &pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_FILE_OPERATION_RESULT,
		Timestamp: time.Now().Unix(),
		Payload:   &pb.NodeEvent_FileResult{FileResult: fileResult},
	}

	if err := stream.Send(event); err != nil {
		a.logger.Error("Failed to send file result", zap.Error(err))
	}
}

// sendCommandResult sends a command result to the controller
func (a *Agent) sendCommandResult(stream *client.EventStream, commandID string, success bool, message string) {
	// Send command result back via the stream
	event := &pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_COMMAND_RESULT,
		Timestamp: time.Now().Unix(),
		Payload: &pb.NodeEvent_CommandResult{
			CommandResult: &pb.CommandResult{
				CommandId: commandID,
				Success:   success,
				Message:   message,
			},
		},
	}

	if err := stream.Send(event); err != nil {
		a.logger.Error("Failed to send command result", zap.Error(err))
	}
}

// sendHeartbeat sends a heartbeat event
func (a *Agent) sendHeartbeat(ctx context.Context, stream *client.EventStream) {
	metrics := a.getCurrentMetrics()
	
	event := &pb.NodeEvent{
		NodeId:    a.cfg.NodeID,
		Type:      pb.EventType_EVENT_TYPE_HEARTBEAT,
		Timestamp: time.Now().Unix(),
		Payload: &pb.NodeEvent_Metrics{
			Metrics: &pb.MetricsSnapshot{
				NodeId:            a.cfg.NodeID,
				CpuUsagePercent:   float32(metrics.CPUUsage),
				MemoryUsagePercent: float32(metrics.MemoryUsage),
				Timestamp:         time.Now().Unix(),
			},
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
