# Game Server Node Agent

A lightweight Go agent for managing game server instances on a single node.

## Features

- **Game Server Management**: Create, start, stop, and delete game servers
- **Resource Monitoring**: Monitor CPU, memory, and network usage
- **Controller Communication**: gRPC bidirectional streaming with controller
- **Process Management**: Safe process spawning and termination
- **Container Ready**: Docker support for easy deployment

## Quick Start

### Prerequisites

- Go 1.21+
- Docker (optional)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/your-org/game-server-node.git
   cd game-server-node
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Configure the node**
   ```bash
   cp config.yaml.example config.yaml
   # Edit config.yaml with your settings
   ```

4. **Run the node agent**
   ```bash
   # Development
   go run ./cmd/node

   # Production (with Docker)
   docker build -t game-server-node:latest .
   docker run -d \
     -e CONTROLLER_ADDRESS=controller:50051 \
     -e NODE_ID=node-1 \
     -v ./servers:/app/servers \
     -v ./backups:/app/backups \
     -v ./logs:/app/logs \
     game-server-node:latest
   ```

## Configuration

```yaml
# Node Identification
node_id: "node-1"
node_name: "Game Node 1"

# Controller Connection
controller_address: "localhost:50051"
heartbeat_interval: 30

# Server Configuration
server_directory: "./servers"
max_servers: 10
default_max_players: 32

# Resources (auto-detected if not set)
total_cpu_cores: 4
total_memory_mb: 8192
```

## Architecture

```
┌────────────────────────────────────────┐
│           Node Agent (Go)              │
│                                        │
│  ┌─────────────┐  ┌─────────────────┐   │
│  │ gRPC Client │  │ Process Manager │   │
│  │ - Register  │  │ - Create       │   │
│  │ - Stream    │  │ - Start/Stop   │   │
│  └─────────────┘  │ - Monitor      │   │
│                   └─────────────────┘   │
│                          │              │
│                    Game Servers         │
└────────────────────────────────────────┘
           │
    gRPC Streaming
           │
           ▼
┌────────────────────────────────────────┐
│           Controller                   │
└────────────────────────────────────────┘
```

## Supported Games

The node agent supports various game types through configurable server templates:

- **Minecraft** - Java Edition servers
- **Valheim** - Dedicated servers
- **Terraria** - TShock servers
- **Factorio** - Dedicated servers
- **Custom** - Generic game servers

## Project Structure

```
game-server-node/
├── cmd/
│   └── node/              # Main entrypoint
├── internal/
│   ├── agent/             # Node agent logic
│   ├── config/            # Configuration
│   ├── grpc/
│   │   ├── client/        # gRPC client
│   │   └── proto/         # Protocol buffers
│   └── process/           # Process management
├── pkg/
│   └── logger/            # Logging utilities
├── config.yaml            # Configuration file
├── Dockerfile
└── README.md
```

## Development

### Building

```bash
# Build binary
go build -o node ./cmd/node

# Build Docker image
docker build -t game-server-node:latest .
```

### Testing

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...
```

## Integration with Controller

The node agent uses gRPC for high-performance communication with the controller:

1. **Registration**: Node registers with controller on startup
2. **Event Streaming**: Bidirectional streaming for commands and events
3. **Heartbeats**: Periodic health updates
4. **Metrics**: Real-time resource utilization data

### Supported Commands

- `CreateServer` - Create a new game server
- `StartServer` - Start a server
- `StopServer` - Stop a server
- `RestartServer` - Restart a server
- `UpdateServer` - Update server configuration
- `DeleteServer` - Delete a server

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

For issues and feature requests, please use the [GitHub Issues](https://github.com/your-org/game-server-node/issues) page.
