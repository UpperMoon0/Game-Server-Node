package process

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/game-server/node/internal/config"
	"go.uber.org/zap"
)

// NodeState represents the persistent state of the node
type NodeState struct {
	Initialized bool   `json:"initialized"`
	GameType    string `json:"game_type"`
}

// Manager manages game server processes and node state
type Manager struct {
	cfg         *config.Config
	logger      *zap.Logger
	servers     map[string]*ServerProcess
	nodeStatus  NodeStatus
	gameType    string
	initialized bool
	stateFile   string
	mu          sync.RWMutex
}

// ServerProcess represents a running game server process
type ServerProcess struct {
	ID          string
	GameType    string
	Version     string
	Config      *ServerConfig
	Process     *exec.Cmd
	Status      ServerStatus
	Port        int
	Output      chan string
	StopChan    chan struct{}
}

// ServerConfig represents the configuration for a game server
type ServerConfig struct {
	ServerID   string
	GameType   string
	Version    string
	Settings   map[string]string
	MaxPlayers int
	Port       int
}

// NodeStatus represents the status of a node
type NodeStatus string

const (
	NodeStatusStopped    NodeStatus = "stopped"    // Default state, node created but not initialized
	NodeStatusInstalling NodeStatus = "installing" // Node is installing dependencies (JDK, etc.)
	NodeStatusRunning    NodeStatus = "running"    // Node is running and ready to host game servers
	NodeStatusError      NodeStatus = "error"      // Node encountered an error
)

// ServerStatus represents the status of a game server
type ServerStatus string

const (
	ServerStatusInstalling ServerStatus = "installing"
	ServerStatusStopped    ServerStatus = "stopped"
	ServerStatusRunning    ServerStatus = "running"
	ServerStatusError      ServerStatus = "error"
	ServerStatusStarting   ServerStatus = "starting"
	ServerStatusStopping   ServerStatus = "stopping"
)

// SystemMetrics represents system metrics
type SystemMetrics struct {
	CPUUsage     float64
	MemoryUsage  float64
	StorageUsage float64
	NetworkIn    int64
	NetworkOut   int64
}

// NewManager creates a new process manager
func NewManager(cfg *config.Config, logger *zap.Logger) (*Manager, error) {
	// Create server directory
	if err := os.MkdirAll(cfg.ServerDirectory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create server directory: %w", err)
	}

	// Create backup directory
	if err := os.MkdirAll(cfg.BackupDirectory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Create data directory for state file
	dataDir := filepath.Join(cfg.ServerDirectory, ".state")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	m := &Manager{
		cfg:       cfg,
		logger:    logger,
		servers:   make(map[string]*ServerProcess),
		stateFile: filepath.Join(dataDir, "node_state.json"),
	}

	// Load persisted state
	if err := m.loadState(); err != nil {
		logger.Warn("Failed to load node state, starting fresh", zap.Error(err))
	}

	return m, nil
}

// loadState loads the persisted node state from disk
func (m *Manager) loadState() error {
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file exists, start fresh
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var state NodeState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	m.initialized = state.Initialized
	m.gameType = state.GameType

	m.logger.Info("Loaded node state from disk",
		zap.Bool("initialized", state.Initialized),
		zap.String("game_type", state.GameType))

	return nil
}

// saveState saves the node state to disk
func (m *Manager) saveState() error {
	state := NodeState{
		Initialized: m.initialized,
		GameType:    m.gameType,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(m.stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// GetNodeStatus returns the current node status
func (m *Manager) GetNodeStatus() NodeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodeStatus
}

// GetGameType returns the node's game type
func (m *Manager) GetGameType() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gameType
}

// IsInitialized returns whether the node has been initialized
func (m *Manager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.initialized
}

// Initialize initializes the node environment based on game type
// This installs dependencies like JDK for Minecraft, etc.
// Returns ErrAlreadyInitialized if the node is already initialized
var ErrAlreadyInitialized = fmt.Errorf("node is already initialized")

func (m *Manager) Initialize(ctx context.Context, gameType string) error {
	// Check if already initialized
	m.mu.RLock()
	if m.initialized {
		m.mu.RUnlock()
		m.logger.Warn("Node is already initialized, skipping initialization",
			zap.String("game_type", m.gameType))
		return ErrAlreadyInitialized
	}
	m.mu.RUnlock()

	m.mu.Lock()
	m.nodeStatus = NodeStatusInstalling
	m.gameType = gameType
	m.mu.Unlock()

	m.logger.Info("Initializing node",
		zap.String("game_type", gameType))

	// Install dependencies based on game type
	switch gameType {
	case "minecraft", "minecraft-java":
		if err := m.installMinecraftDeps(ctx); err != nil {
			m.mu.Lock()
			m.nodeStatus = NodeStatusError
			m.mu.Unlock()
			return fmt.Errorf("failed to install Minecraft dependencies: %w", err)
		}
	case "valheim":
		if err := m.installValheimDeps(ctx); err != nil {
			m.mu.Lock()
			m.nodeStatus = NodeStatusError
			m.mu.Unlock()
			return fmt.Errorf("failed to install Valheim dependencies: %w", err)
		}
	default:
		// For unknown game types, just mark as running
		m.logger.Info("Unknown game type, skipping dependency installation",
			zap.String("game_type", gameType))
	}

	// Set status to running and mark as initialized
	m.mu.Lock()
	m.nodeStatus = NodeStatusRunning
	m.initialized = true
	m.mu.Unlock()

	// Save state to disk
	if err := m.saveState(); err != nil {
		m.logger.Error("Failed to save node state", zap.Error(err))
		// Don't fail the initialization, just log the error
	}

	m.logger.Info("Node initialized successfully",
		zap.String("game_type", gameType),
		zap.String("status", string(NodeStatusRunning)))

	return nil
}

// installMinecraftDeps installs Java JDK for Minecraft servers
func (m *Manager) installMinecraftDeps(ctx context.Context) error {
	m.logger.Info("Installing Minecraft dependencies (JDK 21)")

	// Check if Java is already installed
	if _, err := exec.LookPath("java"); err == nil {
		m.logger.Info("Java is already installed, checking version")
		// Could check version here
		return nil
	}

	// Install OpenJDK 21 (using apt-get for Debian/Ubuntu based containers)
	cmd := exec.CommandContext(ctx, "apt-get", "update")
	if output, err := cmd.CombinedOutput(); err != nil {
		m.logger.Warn("apt-get update failed, continuing anyway",
			zap.Error(err),
			zap.String("output", string(output)))
	}

	cmd = exec.CommandContext(ctx, "apt-get", "install", "-y", "openjdk-21-jre-headless")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install JDK 21: %w, output: %s", err, string(output))
	}

	m.logger.Info("JDK 21 installed successfully")
	return nil
}

// installValheimDeps installs dependencies for Valheim server
func (m *Manager) installValheimDeps(ctx context.Context) error {
	m.logger.Info("Installing Valheim dependencies")

	// Valheim requires some 32-bit libraries and other deps
	// This is a placeholder - actual implementation would install steamcmd, etc.
	
	return nil
}

// CreateServer creates a new game server
func (m *Manager) CreateServer(ctx context.Context, config *ServerConfig) (string, error) {
	// Check if server already exists
	m.mu.RLock()
	if _, exists := m.servers[config.ServerID]; exists {
		m.mu.RUnlock()
		return "", fmt.Errorf("server already exists: %s", config.ServerID)
	}
	m.mu.RUnlock()

	// Create server directory
	serverPath := m.cfg.GetServerPath(config.ServerID)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create server directory: %w", err)
	}

	// Create server process
	server := &ServerProcess{
		ID:       config.ServerID,
		GameType: config.GameType,
		Version:  config.Version,
		Config:   config,
		Status:   ServerStatusStopped,
		Output:   make(chan string, 1000),
		StopChan: make(chan struct{}),
	}

	// Save configuration
	if err := m.saveConfig(serverPath, config); err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}

	// Store server
	m.mu.Lock()
	m.servers[config.ServerID] = server
	m.mu.Unlock()

	m.logger.Info("Server created",
		zap.String("server_id", config.ServerID),
		zap.String("game_type", config.GameType))

	return config.ServerID, nil
}

// StartServer starts a game server
func (m *Manager) StartServer(ctx context.Context, serverID string) error {
	m.mu.Lock()
	server, exists := m.servers[serverID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("server not found: %s", serverID)
	}

	if server.Status == ServerStatusRunning {
		m.mu.Unlock()
		return fmt.Errorf("server already running: %s", serverID)
	}

	server.Status = ServerStatusStarting
	m.mu.Unlock()

	// Start the server process
	if err := m.startProcess(server); err != nil {
		server.Status = ServerStatusError
		return fmt.Errorf("failed to start server: %w", err)
	}

	m.logger.Info("Server started",
		zap.String("server_id", serverID),
		zap.String("game_type", server.GameType))

	return nil
}

// StopServer stops a game server
func (m *Manager) StopServer(ctx context.Context, serverID string) error {
	m.mu.Lock()
	server, exists := m.servers[serverID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("server not found: %s", serverID)
	}

	if server.Status == ServerStatusStopped {
		m.mu.Unlock()
		return nil
	}

	server.Status = ServerStatusStopping
	m.mu.Unlock()

	// Stop the server process
	if err := m.stopProcess(server); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	server.Status = ServerStatusStopped

	m.logger.Info("Server stopped", zap.String("server_id", serverID))

	return nil
}

// RestartServer restarts a game server
func (m *Manager) RestartServer(ctx context.Context, serverID string) error {
	// Stop server
	if err := m.StopServer(ctx, serverID); err != nil {
		return err
	}

	// Wait for stop to complete
	time.Sleep(2 * time.Second)

	// Start server
	return m.StartServer(ctx, serverID)
}

// DeleteServer deletes a game server
func (m *Manager) DeleteServer(ctx context.Context, serverID string) error {
	m.mu.Lock()
	server, exists := m.servers[serverID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("server not found: %s", serverID)
	}
	m.mu.Unlock()

	// Stop server if running
	if server.Status == ServerStatusRunning {
		m.StopServer(ctx, serverID)
	}

	// Remove from map
	m.mu.Lock()
	delete(m.servers, serverID)
	m.mu.Unlock()

	// Remove server directory
	serverPath := m.cfg.GetServerPath(serverID)
	if err := os.RemoveAll(serverPath); err != nil {
		return fmt.Errorf("failed to remove server directory: %w", err)
	}

	m.logger.Info("Server deleted", zap.String("server_id", serverID))

	return nil
}

// StopAll stops all running servers
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, server := range m.servers {
		if server.Status == ServerStatusRunning {
			m.stopProcess(server)
		}
	}

	return nil
}

// GetServer returns a server by ID
func (m *Manager) GetServer(serverID string) (*ServerProcess, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	server, exists := m.servers[serverID]
	if !exists {
		return nil, fmt.Errorf("server not found: %s", serverID)
	}

	return server, nil
}

// GetAllServers returns all servers
func (m *Manager) GetAllServers() []*ServerProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	servers := make([]*ServerProcess, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}

	return servers
}

// UpdateMetrics updates server metrics
func (m *Manager) UpdateMetrics(metrics *SystemMetrics) {
	// TODO: Implement metrics update
}

// GetSystemMetrics returns current system metrics
func (m *Manager) GetSystemMetrics() *SystemMetrics {
	return &SystemMetrics{
		CPUUsage:     0, // TODO: Get actual CPU usage
		MemoryUsage:  0,
		StorageUsage: 0,
	}
}

// CheckHealth checks the health of all servers
func (m *Manager) CheckHealth() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, server := range m.servers {
		if server.Status == ServerStatusRunning && server.Process != nil {
			if server.Process.Process == nil {
				server.Status = ServerStatusError
				m.logger.Error("Server process died unexpectedly",
					zap.String("server_id", server.ID))
			}
		}
	}
}

// startProcess starts a server process
func (m *Manager) startProcess(server *ServerProcess) error {
	// TODO: Implement actual game server startup
	// This would be different for each game type
	
	serverPath := m.cfg.GetServerPath(server.ID)
	
	// Create a dummy process for now
	cmd := exec.CommandContext(context.Background(), "sleep", "3600")
	cmd.Dir = serverPath

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	server.Process = cmd
	server.Status = ServerStatusRunning

	return nil
}

// stopProcess stops a server process
func (m *Manager) stopProcess(server *ServerProcess) error {
	if server.Process != nil && server.Process.Process != nil {
		if err := server.Process.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	close(server.StopChan)
	return nil
}

// saveConfig saves the server configuration
func (m *Manager) saveConfig(serverPath string, config *ServerConfig) error {
	// TODO: Save config to file
	_ = filepath.Join(serverPath, "config.json")
	return nil
}
