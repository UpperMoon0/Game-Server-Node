package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/nstut/game-server-proto/gen"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Client represents a gRPC client for communicating with the controller
type Client struct {
	conn    *grpc.ClientConn
	client  pb.NodeServiceClient
	logger  *zap.Logger
	address string
}

// EventStream represents a bidirectional event stream
type EventStream struct {
	stream pb.NodeService_StreamEventsClient
}

// NewClient creates a new gRPC client
func NewClient(address string, logger *zap.Logger) (*Client, error) {
	// Create connection with insecure credentials (use TLS in production)
	// Use keepalive to detect dead connections
	keepaliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second, // Send keepalive ping every 10s
		Timeout:             5 * time.Second,  // Wait 5s for response
		PermitWithoutStream: true,             // Send pings even without active streams
	}

	conn, err := grpc.Dial(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepaliveParams),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to controller: %w", err)
	}

	return &Client{
		conn:    conn,
		client:  pb.NewNodeServiceClient(conn),
		logger:  logger,
		address: address,
	}, nil
}

// Reconnect closes the existing connection and creates a new one
func (c *Client) Reconnect() error {
	if c.conn != nil {
		c.conn.Close()
	}

	keepaliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.Dial(c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepaliveParams),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to reconnect to controller: %w", err)
	}

	c.conn = conn
	c.client = pb.NewNodeServiceClient(conn)
	return nil
}

// Close closes the gRPC connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// RegisterNode registers the node with the controller
func (c *Client) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.RegisterNode(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to register node: %w", err)
	}

	if resp == nil {
		return nil, fmt.Errorf("received nil response from controller")
	}

	if c.logger != nil {
		c.logger.Info("Successfully registered with controller",
			zap.String("controller_id", resp.ControllerId))
	}

	return resp, nil
}

// StreamEvents creates a bidirectional event stream
func (c *Client) StreamEvents(ctx context.Context) (*EventStream, error) {
	stream, err := c.client.StreamEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create event stream: %w", err)
	}

	return &EventStream{stream: stream}, nil
}

// Send sends an event through the stream
func (s *EventStream) Send(event *pb.NodeEvent) error {
	return s.stream.Send(event)
}

// Recv receives a command from the stream
func (s *EventStream) Recv() (*pb.ControllerCommand, error) {
	cmd, err := s.stream.Recv()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// GetServerService returns a server service client
func (c *Client) GetServerService() pb.ServerServiceClient {
	return pb.NewServerServiceClient(c.conn)
}

// GetMetricsService returns a metrics service client
func (c *Client) GetMetricsService() pb.MetricsServiceClient {
	return pb.NewMetricsServiceClient(c.conn)
}

// NeedsReconnect checks if the connection needs to be reestablished
func (c *Client) NeedsReconnect() bool {
	if c.conn == nil {
		return true
	}
	// Check if connection is in a valid state
	state := c.conn.GetState()
	return state == connectivity.Shutdown || state == connectivity.TransientFailure
}
