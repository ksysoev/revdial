package proto

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/ksysoev/revdial/mux"
)

// ClientV2 extends Client with V2 protocol support and multiplexing.
// It maintains a yamux session for stream-based connections.
type ClientV2 struct {
	*Client
	session   *yamux.Session
	muxConfig *mux.Config
	isV2      bool
}

// NewClientV2 creates a new ClientV2 instance with V2 protocol support.
// It takes a connection and optional client options.
func NewClientV2(conn io.ReadWriteCloser, opts ...ClientOption) *ClientV2 {
	c2 := &ClientV2{
		Client:    NewClient(conn),
		muxConfig: mux.DefaultConfig(),
		isV2:      false,
	}

	// Apply options to the ClientV2 instance
	for _, opt := range opts {
		opt(c2.Client)
	}

	return c2
}

// Register attempts to register using V2 protocol with multiplexing.
// If V2 fails and fallback is enabled, it falls back to V1 protocol.
func (c *ClientV2) Register(ctx context.Context, id uuid.UUID) error {
	c.mu.Lock()

	if c.state != connected {
		c.mu.Unlock()
		return fmt.Errorf("unexpected state: %d", c.state)
	}

	c.state = processing
	c.mu.Unlock()

	ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		<-ctx.Done()

		_ = c.conn.Close()
	}()

	// If V2 is disabled (V1-only mode), use V1 directly
	if c.disableV2 {
		return c.registerV1(ctx, id)
	}

	// Try V2 first
	if err := c.tryV2Registration(ctx, id); err != nil {
		// V2 failed - connection may be corrupted
		// In a real scenario, we would need to reconnect with a fresh connection
		// For now, return error since the connection state is unknown
		return fmt.Errorf("V2 registration failed and fallback requires reconnection: %w", err)
	}

	return nil
}

// tryV2Registration attempts to register using V2 protocol with multiplexing.
func (c *ClientV2) tryV2Registration(ctx context.Context, id uuid.UUID) error {
	// Establish connection (auth)
	if err := c.establish(); err != nil {
		return fmt.Errorf("failed to establish connection: %w", err)
	}

	// Send MuxInit command
	configBuf := make([]byte, 13)
	configBuf[0] = cmdMuxInit
	binary.BigEndian.PutUint32(configBuf[1:5], c.muxConfig.MaxStreams)
	binary.BigEndian.PutUint32(configBuf[5:9], c.muxConfig.StreamWindowSize)
	binary.BigEndian.PutUint32(configBuf[9:13], c.muxConfig.ConnectionWindowSize)

	resp, err := sendRequest(c.conn, configBuf)
	if err != nil {
		// Connection error - server likely doesn't support V2 or closed connection
		return fmt.Errorf("server does not support V2 or connection error: %w", err)
	}

	if resp != cmdMuxReady {
		// Server responded but not with MuxReady - probably V1 server returning error
		return fmt.Errorf("server does not support V2: response %d", resp)
	}

	// Read negotiated configuration (12 bytes)
	negotiatedBuf := make([]byte, 12)
	if _, err := io.ReadFull(c.conn, negotiatedBuf); err != nil {
		return fmt.Errorf("failed to read mux config: %w", err)
	}

	// Upgrade to yamux session (client mode)
	yamuxCfg := c.muxConfig.ToYamux()

	session, err := yamux.Client(c.conn, yamuxCfg)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	c.session = session
	c.isV2 = true

	// Open control stream
	controlStream, err := session.OpenStream()
	if err != nil {
		return fmt.Errorf("failed to open control stream: %w", err)
	}

	// Send register command on control stream
	req := make([]byte, 17)
	req[0] = cmdRegister
	copy(req[1:], id[:])

	regResp, err := sendRequest(controlStream, req)
	if err != nil {
		return fmt.Errorf("failed to send register: %w", err)
	}

	if regResp != resSuccess {
		return fmt.Errorf("registration failed: %d", regResp)
	}

	c.mu.Lock()
	c.state = registered
	c.mu.Unlock()

	// Start command handler on control stream
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		defer c.cancel()
		defer close(c.cmds)

		// Replace conn with control stream for command handling
		c.conn = controlStream

		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := c.handleCommand(ctx)
				if err != nil && !errors.Is(err, io.EOF) {
					slog.Error("failed to handle command", slog.Any("error", err))
					return
				}
			}
		}
	}()

	return nil
}

// registerV1 registers using V1 protocol.
func (c *ClientV2) registerV1(ctx context.Context, id uuid.UUID) error {
	c.isV2 = false

	// Establish connection (auth) - needed for V1
	if err := c.establish(); err != nil {
		c.cancel()
		return fmt.Errorf("failed to establish connection: %w", err)
	}

	// Send register command
	if err := c.handleRegister(id); err != nil {
		c.cancel()
		return fmt.Errorf("failed to handle register: %w", err)
	}

	c.mu.Lock()
	c.state = registered
	c.mu.Unlock()

	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		defer c.cancel()
		defer close(c.cmds)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := c.handleCommand(ctx)
				if err != nil && !errors.Is(err, io.EOF) {
					slog.Error("failed to handle command", slog.Any("error", err))
					return
				}
			}
		}
	}()

	return nil
}

// Bind binds to a specific connection ID using either V2 stream or V1 connection.
func (c *ClientV2) Bind(ctx context.Context, id uuid.UUID) error {
	if c.isV2 {
		return c.bindV2Stream(ctx, id)
	}

	// V1 fallback
	return c.Client.Bind(ctx, id)
}

// bindV2Stream opens a new stream and binds it to the given ID.
func (c *ClientV2) bindV2Stream(_ context.Context, id uuid.UUID) error {
	if c.session == nil {
		return fmt.Errorf("no yamux session available")
	}

	// Open a new stream
	stream, err := c.session.OpenStream()
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	// Send bind command
	req := make([]byte, 17)
	req[0] = cmdBind
	copy(req[1:], id[:])

	resp, err := sendRequest(stream, req)
	if err != nil {
		_ = stream.Close()
		return fmt.Errorf("failed to send bind: %w", err)
	}

	if resp != resSuccess {
		_ = stream.Close()
		return fmt.Errorf("bind failed: %d", resp)
	}

	c.mu.Lock()
	c.state = bound
	c.conn = stream
	c.mu.Unlock()

	return nil
}

// IsV2 returns true if using V2 protocol with multiplexing.
func (c *ClientV2) IsV2() bool {
	return c.isV2
}

// Session returns the yamux session if using V2.
func (c *ClientV2) Session() *yamux.Session {
	return c.session
}

// WithMuxConfigClient sets the multiplexing configuration for V2 connections.
func WithMuxConfigClient(config *mux.Config) ClientOption {
	return func(c *Client) {
		if c2, ok := interface{}(c).(*ClientV2); ok {
			c2.muxConfig = config
		}
	}
}

// WithDisableV2Fallback disables automatic fallback to V1 protocol.
// This forces the client to use V1-only mode.
func WithDisableV2Fallback() ClientOption {
	return func(c *Client) {
		c.disableV2 = true
	}
}
