package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/game-server/node/internal/grpc/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client represents a gRPC client for communicating with the controller
type Client struct {
	conn    *grpc.ClientConn
	client  pb.NodeServiceClient
	logger  *zap.Logger
}

// EventStream represents a bidirectional event stream
type EventStream struct {
	stream  pb.NodeService_StreamEventsClient
}

// NewClient creates a new gRPC client
func NewClient(address string, logger *zap.Logger) (*Client, error) {
	// Create connection with insecure credentials (use TLS in production)
	conn, err := grpc.Dial(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to controller: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewNodeServiceClient(conn),
		logger: logger,
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// RegisterNode registers the node with the controller
func (c *Client) RegisterNode(ctx context.Context, nodeInfo *pb.NodeInfo) (*pb.RegisterNodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.RegisterNode(ctx, nodeInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to register node: %w", err)
	}

	c.logger.Info("Successfully registered with controller",
		zap.String("controller_id", resp.ControllerId))

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
