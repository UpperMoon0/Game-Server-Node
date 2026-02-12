package config

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/viper"
)

// Config holds all configuration for the node agent
type Config struct {
	// Node Identification
	NodeID   string `mapstructure:"NODE_ID"`
	NodeName string `mapstructure:"NODE_NAME"`

	// Controller Connection
	ControllerAddress string `mapstructure:"CONTROLLER_ADDRESS"`
	GRPCAddress        string `mapstructure:"GRPC_ADDRESS"`
	HeartbeatInterval  int    `mapstructure:"HEARTBEAT_INTERVAL"`
	NodeTimeout        int    `mapstructure:"NODE_TIMEOUT"`

	// Server Configuration
	ServerDirectory    string `mapstructure:"SERVER_DIRECTORY"`
	MaxServers         int    `mapstructure:"MAX_SERVERS"`
	DefaultMaxPlayers  int    `mapstructure:"DEFAULT_MAX_PLAYERS"`
	BackupDirectory    string `mapstructure:"BACKUP_DIRECTORY"`

	// Resources
	TotalCPUCores    int   `mapstructure:"TOTAL_CPU_CORES"`
	TotalMemoryMB    int64 `mapstructure:"TOTAL_MEMORY_MB"`
	TotalStorageMB   int64 `mapstructure:"TOTAL_STORAGE_MB"`

	// Logging
	LogLevel    string `mapstructure:"LOG_LEVEL"`
	LogFormat   string `mapstructure:"LOG_FORMAT"`
	LogFilePath string `mapstructure:"LOG_FILE_PATH"`

	// Environment
	Environment string `mapstructure:"ENVIRONMENT"`
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("NODE_NAME", "node-"+string(runtime.GOOS))
	v.SetDefault("HEARTBEAT_INTERVAL", 30)
	v.SetDefault("NODE_TIMEOUT", 120)
	v.SetDefault("SERVER_DIRECTORY", "./servers")
	v.SetDefault("MAX_SERVERS", 10)
	v.SetDefault("DEFAULT_MAX_PLAYERS", 32)
	v.SetDefault("BACKUP_DIRECTORY", "./backups")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")
	v.SetDefault("LOG_FILE_PATH", "./logs/node.log")
	v.SetDefault("ENVIRONMENT", "development")

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/game-server-node")
	}

	// Environment variables
	v.SetEnvPrefix("GSN")
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply environment overrides
	if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
		cfg.NodeID = nodeID
	}
	if controllerAddr := os.Getenv("CONTROLLER_ADDRESS"); controllerAddr != "" {
		cfg.ControllerAddress = controllerAddr
	}

	// Auto-detect resources if not set
	if cfg.TotalCPUCores == 0 {
		cfg.TotalCPUCores = runtime.NumCPU()
	}
	if cfg.TotalMemoryMB == 0 {
		// Estimate available memory (this is a rough estimate)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		cfg.TotalMemoryMB = int64(m.Sys) / (1024 * 1024)
	}

	// Generate node ID if not set
	if cfg.NodeID == "" {
		cfg.NodeID = fmt.Sprintf("node-%d", os.Getpid())
	}

	return &cfg, nil
}

// GetHeartbeatInterval returns the heartbeat interval as seconds
func (c *Config) GetHeartbeatInterval() int {
	if c.HeartbeatInterval == 0 {
		return 30
	}
	return c.HeartbeatInterval
}

// GetNodeTimeout returns the node timeout as seconds
func (c *Config) GetNodeTimeout() int {
	if c.NodeTimeout == 0 {
		return 120
	}
	return c.NodeTimeout
}

// GetServerPath returns the path to a server directory
func (c *Config) GetServerPath(serverID string) string {
	return fmt.Sprintf("%s/%s", c.ServerDirectory, serverID)
}

// GetBackupPath returns the path to a backup directory
func (c *Config) GetBackupPath(serverID string) string {
	return fmt.Sprintf("%s/%s", c.BackupDirectory, serverID)
}
