package proto

//go:generate protoc --go_out=. --go-grpc_out=. controller.proto

// EventType represents the type of node event
type EventType int32

const (
	EventType_EVENT_TYPE_UNSPECIFIED        EventType = 0
	EventType_EVENT_TYPE_NODE_ONLINE        EventType = 1
	EventType_EVENT_TYPE_NODE_OFFLINE       EventType = 2
	EventType_EVENT_TYPE_HEARTBEAT          EventType = 3
	EventType_EVENT_TYPE_NODE_INITIALIZING  EventType = 4
	EventType_EVENT_TYPE_NODE_READY         EventType = 5
	EventType_EVENT_TYPE_SERVER_CREATED     EventType = 6
	EventType_EVENT_TYPE_SERVER_STARTED     EventType = 7
	EventType_EVENT_TYPE_SERVER_STOPPED     EventType = 8
	EventType_EVENT_TYPE_SERVER_RESTARTED   EventType = 9
	EventType_EVENT_TYPE_SERVER_DELETED     EventType = 10
	EventType_EVENT_TYPE_METRICS_REPORT     EventType = 11
)

func (e EventType) String() string {
	switch e {
	case EventType_EVENT_TYPE_NODE_ONLINE:
		return "NODE_ONLINE"
	case EventType_EVENT_TYPE_NODE_OFFLINE:
		return "NODE_OFFLINE"
	case EventType_EVENT_TYPE_HEARTBEAT:
		return "HEARTBEAT"
	case EventType_EVENT_TYPE_NODE_INITIALIZING:
		return "NODE_INITIALIZING"
	case EventType_EVENT_TYPE_NODE_READY:
		return "NODE_READY"
	case EventType_EVENT_TYPE_SERVER_CREATED:
		return "SERVER_CREATED"
	case EventType_EVENT_TYPE_SERVER_STARTED:
		return "SERVER_STARTED"
	case EventType_EVENT_TYPE_SERVER_STOPPED:
		return "SERVER_STOPPED"
	case EventType_EVENT_TYPE_SERVER_RESTARTED:
		return "SERVER_RESTARTED"
	case EventType_EVENT_TYPE_SERVER_DELETED:
		return "SERVER_DELETED"
	case EventType_EVENT_TYPE_METRICS_REPORT:
		return "METRICS_REPORT"
	default:
		return "UNSPECIFIED"
	}
}

// CommandType represents the type of controller command
type CommandType int32

const (
	CommandType_COMMAND_TYPE_UNSPECIFIED     CommandType = 0
	CommandType_COMMAND_TYPE_INITIALIZE_NODE CommandType = 1
	CommandType_COMMAND_TYPE_CREATE_SERVER   CommandType = 2
	CommandType_COMMAND_TYPE_START_SERVER    CommandType = 3
	CommandType_COMMAND_TYPE_STOP_SERVER     CommandType = 4
	CommandType_COMMAND_TYPE_RESTART_SERVER  CommandType = 5
	CommandType_COMMAND_TYPE_UPDATE_SERVER   CommandType = 6
	CommandType_COMMAND_TYPE_DELETE_SERVER   CommandType = 7
)

func (c CommandType) String() string {
	switch c {
	case CommandType_COMMAND_TYPE_INITIALIZE_NODE:
		return "INITIALIZE_NODE"
	case CommandType_COMMAND_TYPE_CREATE_SERVER:
		return "CREATE_SERVER"
	case CommandType_COMMAND_TYPE_START_SERVER:
		return "START_SERVER"
	case CommandType_COMMAND_TYPE_STOP_SERVER:
		return "STOP_SERVER"
	case CommandType_COMMAND_TYPE_RESTART_SERVER:
		return "RESTART_SERVER"
	case CommandType_COMMAND_TYPE_UPDATE_SERVER:
		return "UPDATE_SERVER"
	case CommandType_COMMAND_TYPE_DELETE_SERVER:
		return "DELETE_SERVER"
	default:
		return "UNSPECIFIED"
	}
}

// ServerState represents the state of a game server
type ServerState int32

const (
	ServerState_SERVER_STATE_UNSPECIFIED ServerState = 0
	ServerState_SERVER_STATE_STOPPED     ServerState = 1
	ServerState_SERVER_STATE_RUNNING    ServerState = 2
	ServerState_SERVER_STATE_STARTING   ServerState = 3
	ServerState_SERVER_STATE_STOPPING  ServerState = 4
	ServerState_SERVER_STATE_ERROR      ServerState = 5
)

func (s ServerState) String() string {
	switch s {
	case ServerState_SERVER_STATE_STOPPED:
		return "STOPPED"
	case ServerState_SERVER_STATE_RUNNING:
		return "RUNNING"
	case ServerState_SERVER_STATE_STARTING:
		return "STARTING"
	case ServerState_SERVER_STATE_STOPPING:
		return "STOPPING"
	case ServerState_SERVER_STATE_ERROR:
		return "ERROR"
	default:
		return "UNSPECIFIED"
	}
}

// NodeInfo represents information about a node
type NodeInfo struct {
	NodeId          string          `json:"nodeId"`
	Hostname        string          `json:"hostname"`
	IpAddress       string          `json:"ipAddress"`
	Resources       *NodeResources  `json:"resources"`
	GameTypes       []string        `json:"gameTypes"`
	OsVersion       string          `json:"osVersion"`
	AgentVersion    string          `json:"agentVersion"`
	Initialized     bool            `json:"initialized"`
	GameType        string          `json:"gameType"`
}

// NodeResources represents resources available on a node
type NodeResources struct {
	TotalCpuCores        int32   `json:"totalCpuCores"`
	TotalMemoryMb        int64   `json:"totalMemoryMb"`
	TotalStorageMb       int64   `json:"totalStorageMb"`
	AvailableCpuCores   int32   `json:"availableCpuCores"`
	AvailableMemoryMb   int64   `json:"availableMemoryMb"`
	AvailableStorageMb  int64   `json:"availableStorageMb"`
}

// NodeEvent represents an event from a node
type NodeEvent struct {
	NodeId          string          `json:"nodeId"`
	Type            EventType       `json:"type"`
	Timestamp       int64           `json:"timestamp"`
	ServerStatus    *ServerStatus   `json:"serverStatus,omitempty"`
	Metrics         *MetricsSnapshot `json:"metrics,omitempty"`
}

// ControllerCommand represents a command from the controller
type ControllerCommand struct {
	CommandId       string           `json:"commandId"`
	Type            CommandType      `json:"type"`
	InitializeNode  *InitializeNodeCmd `json:"initializeNode,omitempty"`
	CreateServer    *CreateServerCmd `json:"createServer,omitempty"`
	StartServer     *StartServerCmd  `json:"startServer,omitempty"`
	StopServer      *StopServerCmd   `json:"stopServer,omitempty"`
	DeleteServer    *DeleteServerCmd `json:"deleteServer,omitempty"`
}

// GetInitializeNode returns the InitializeNode command if present
func (c *ControllerCommand) GetInitializeNode() *InitializeNodeCmd {
	if c != nil {
		return c.InitializeNode
	}
	return nil
}

// GetCreateServer returns the CreateServer command if present
func (c *ControllerCommand) GetCreateServer() *CreateServerCmd {
	if c != nil {
		return c.CreateServer
	}
	return nil
}

// GetStartServer returns the StartServer command if present
func (c *ControllerCommand) GetStartServer() *StartServerCmd {
	if c != nil {
		return c.StartServer
	}
	return nil
}

// GetStopServer returns the StopServer command if present
func (c *ControllerCommand) GetStopServer() *StopServerCmd {
	if c != nil {
		return c.StopServer
	}
	return nil
}

// GetDeleteServer returns the DeleteServer command if present
func (c *ControllerCommand) GetDeleteServer() *DeleteServerCmd {
	if c != nil {
		return c.DeleteServer
	}
	return nil
}

// CreateServerCmd contains parameters for creating a server
type CreateServerCmd struct {
	ServerId    string          `json:"serverId"`
	GameType    string          `json:"gameType"`
	Config      *ServerConfig   `json:"config"`
}

// StartServerCmd contains parameters for starting a server
type StartServerCmd struct {
	ServerId    string  `json:"serverId"`
}

// StopServerCmd contains parameters for stopping a server
type StopServerCmd struct {
	ServerId    string  `json:"serverId"`
}

// DeleteServerCmd contains parameters for deleting a server
type DeleteServerCmd struct {
	ServerId    string  `json:"serverId"`
}

// InitializeNodeCmd contains parameters for initializing a node
type InitializeNodeCmd struct {
	GameType    string  `json:"gameType"`
}

// ServerConfig contains configuration for a game server
type ServerConfig struct {
	Version     string            `json:"version"`
	Settings    map[string]string `json:"settings"`
	MaxPlayers  int32             `json:"maxPlayers"`
}

// ServerStatus represents the status of a game server
type ServerStatus struct {
	ServerId      string      `json:"serverId"`
	State         ServerState `json:"state"`
}

// MetricsSnapshot represents a snapshot of node metrics
type MetricsSnapshot struct {
	NodeId              string  `json:"nodeId"`
	CpuUsagePercent     float32 `json:"cpuUsagePercent"`
	MemoryUsagePercent  float32 `json:"memoryUsagePercent"`
	Timestamp           int64   `json:"timestamp"`
}

// RegisterNodeResponse represents the response from registering a node
type RegisterNodeResponse struct {
	ControllerId           string  `json:"controllerId"`
	HeartbeatIntervalSeconds int64  `json:"heartbeatIntervalSeconds"`
}

// Stub implementations for gRPC clients (to be replaced with actual generated code)

type nodeServiceClient struct{}

func (c *nodeServiceClient) RegisterNode(ctx interface{}, nodeInfo *NodeInfo) (*RegisterNodeResponse, error) {
	return nil, nil
}

func (c *nodeServiceClient) StreamEvents(ctx interface{}) (NodeService_StreamEventsClient, error) {
	return nil, nil
}

// NodeServiceClient is the client API for NodeService service.
type NodeServiceClient interface {
	RegisterNode(ctx interface{}, nodeInfo *NodeInfo) (*RegisterNodeResponse, error)
	StreamEvents(ctx interface{}) (NodeService_StreamEventsClient, error)
}

// NewNodeServiceClient creates a new NodeService client
func NewNodeServiceClient(cc interface{}) NodeServiceClient {
	return &nodeServiceClient{}
}

// ServerServiceClient is the client API for ServerService service.
type ServerServiceClient interface{}

// NewServerServiceClient creates a new ServerService client
func NewServerServiceClient(cc interface{}) ServerServiceClient {
	return nil
}

// MetricsServiceClient is the client API for MetricsService service.
type MetricsServiceClient interface{}

// NewMetricsServiceClient creates a new MetricsService client
func NewMetricsServiceClient(cc interface{}) MetricsServiceClient {
	return nil
}

// NodeService_StreamEventsClient is the client API for NodeService_StreamEvents service.
type NodeService_StreamEventsClient interface {
	Send(*NodeEvent) error
	Recv() (*ControllerCommand, error)
}
