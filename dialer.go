package revdial

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/connmng"
	"github.com/ksysoev/revdial/proto"
)

type connRequest struct {
	ctx context.Context
	ch  chan net.Conn
}

type Dialer struct {
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	cm         *connmng.ConnManager
	requests   map[uuid.UUID]*connRequest
	tlsConfig  *tls.Config
	listen     string
	serverOpts []proto.ServerOption
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

type DialerOption func(*Dialer)

// NewDialer creates and returns a new instance of Dialer configured with the specified listen address.
// It takes listen of type string, specifying the address to bind to, and optional DialerOption arguments to customize the Dialer.
// It returns a pointer to the newly created Dialer instance.
func NewDialer(listen string, opts ...DialerOption) *Dialer {
	d := &Dialer{
		listen:   listen,
		cm:       connmng.New(),
		requests: make(map[uuid.UUID]*connRequest),
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Start initializes and starts the Dialer, beginning to listen for incoming connections.
// It takes a Context (ctx) to manage the lifetime of the listener and associated goroutines.
// It returns an error if the listener fails to start or an issue occurs during initialization.
func (d *Dialer) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ctx, d.cancel = context.WithCancel(ctx)

	var (
		l   net.Listener
		err error
	)

	if d.tlsConfig != nil {
		l, err = tls.Listen("tcp", d.listen, d.tlsConfig)
	} else {
		l, err = net.Listen("tcp", d.listen)
	}

	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	d.listener = l

	d.wg.Add(2)

	go func() {
		defer d.wg.Done()

		d.serve(d.ctx)
	}()

	go func() {
		defer d.wg.Done()

		<-d.ctx.Done()

		_ = d.listener.Close()
	}()

	return nil
}

// Addr returns the address that the Dialer is currently listening on as a string.
// It does not take any parameters and returns the network address of the listener.
// If the listener is not initialized, it may return an empty string.
func (d *Dialer) Addr() string {
	return d.listener.Addr().String()
}

// Stop gracefully shuts down the Dialer, stopping the listener and associated goroutines.
// It does not take any parameters and returns an error if the listener fails to close.
// It safely handles cases where Stop is called without prior initialization or a running listener.
func (d *Dialer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cancel == nil {
		return nil
	}

	d.cancel()
	d.wg.Wait()

	return nil
}

// DialContext establishes a new connection using the context for control and cancellation.
// It takes a ctx of type context.Context to manage connection lifecycle.
// It returns a net.Conn representing the connection and an error if no connection is available or other issues occur.
func (d *Dialer) DialContext(ctx context.Context) (net.Conn, error) {
	s := d.cm.GetConn()

	if s == nil || s.State() != proto.StateRegistered {
		return nil, fmt.Errorf("no connection is available")
	}

	id := uuid.New()

	ch := d.addRequest(ctx, id)
	defer d.removeRequest(id)

	err := s.SendConnectCommand(id)
	if err != nil {
		return nil, fmt.Errorf("failed to request connection: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-ch:
		return conn, nil
	}
}

// SendEvent sends a custom event with the given name and payload through an active connection.
// It takes a context.Context (unused), a name of type string, and a payload of type any.
// It returns an error if no active connection is available or if sending the event fails.
func (d *Dialer) SendEvent(_ context.Context, name string, payload any) error {
	s := d.cm.GetConn()

	if s == nil || s.State() != proto.StateRegistered {
		return fmt.Errorf("no connection is available")
	}

	err := s.SendCustomEvent(name, payload)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}

	return nil
}

// serve handles incoming connections on the Dialer's listener and processes them based on their state.
// It takes a Context (ctx) to manage the lifetime of the serve routine.
// It does not return any values directly but terminates if the listener is closed or ctx is canceled.
// It stops processing a connection in case of errors during its handling.
func (d *Dialer) serve(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			cancel()
			return
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			d.handleConnection(ctx, conn)
		}()
	}
}

// handleConnection manages a single incoming connection and processes its state.
// It takes ctx of type context.Context for managing the lifecycle of the operation and conn of type net.Conn for the connection.
// It does not return any value. If an error occurs during state processing, the connection is closed or ignored.
// It handles different states (Registered, Bound) and adds or removes connections to/from the connection manager.
// It supports both V1 and V2 protocols with automatic detection.
func (d *Dialer) handleConnection(ctx context.Context, conn net.Conn) {
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	defer cancel()

	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	// Use ServerV2 which handles both V1 and V2 protocols
	s := proto.NewServerV2(conn, d.serverOpts)
	if err := s.Process(); err != nil {
		return
	}

	switch s.State() {
	case proto.StateRegistered:
		// For V2 connections, we need to handle streams differently
		if s.IsV2() {
			d.handleV2RegisteredConnection(ctx, s)
		} else {
			// V1 connection - add to connection manager
			d.cm.AddConnection(s)
		}

		close(done)

		return
	case proto.StateBound:
		id := s.ID()
		req := d.removeRequest(id)

		if req == nil {
			return
		}

		select {
		case req.ch <- conn:
			close(done)
			return
		case <-req.ctx.Done():
			return
		}

	default:
		slog.Error("unexpected state while handling incoming connection", slog.Any("state", s.State()))
		return
	}
}

// handleV2RegisteredConnection handles a registered V2 connection with multiplexing support.
// It spawns a goroutine to accept incoming streams and match them to connection requests.
func (d *Dialer) handleV2RegisteredConnection(_ context.Context, s *proto.ServerV2) {
	// Add the V2 server to connection manager (it implements ServerConn interface)
	d.cm.AddConnection(s)

	// Start accepting streams in the background
	d.wg.Add(1)

	go func() {
		defer d.wg.Done()
		defer d.cm.RemoveConnection(s.ID())

		for {
			select {
			case <-d.ctx.Done():
				return
			default:
				stream, err := s.AcceptStream()
				if err != nil {
					slog.Error("failed to accept stream", slog.Any("error", err))
					return
				}

				// Read the bind command from the stream
				go d.handleV2Stream(d.ctx, stream)
			}
		}
	}()
}

// handleV2Stream handles an incoming stream in V2 mode by reading the bind command
// and matching it to a pending connection request.
func (d *Dialer) handleV2Stream(ctx context.Context, stream net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			_ = stream.Close()
		}
	}()

	// Read version and command
	buf := make([]byte, 2)
	if _, err := stream.Read(buf); err != nil {
		slog.Error("failed to read command from stream", slog.Any("error", err))

		_ = stream.Close()

		return
	}

	// V2 streams use V1 format for compatibility
	if buf[0] != 1 { // versionV1
		slog.Error("unexpected version in stream", slog.Int("version", int(buf[0])))

		_ = stream.Close()

		return
	}

	if buf[1] != proto.CmdBind() {
		slog.Error("unexpected command in stream", slog.Int("command", int(buf[1])))

		_ = stream.Close()

		return
	}

	// Read UUID (16 bytes)
	uuidBuf := make([]byte, 16)
	if _, err := stream.Read(uuidBuf); err != nil {
		slog.Error("failed to read UUID from stream", slog.Any("error", err))

		_ = stream.Close()

		return
	}

	id, err := uuid.FromBytes(uuidBuf)
	if err != nil {
		slog.Error("failed to parse UUID", slog.Any("error", err))

		_ = stream.Close()

		return
	}

	// Send success response (using V1 format for compatibility)
	if _, err := stream.Write([]byte{proto.VersionV1(), proto.ResSuccess()}); err != nil {
		slog.Error("failed to write response", slog.Any("error", err))

		_ = stream.Close()

		return
	}

	// Find the pending request and send the stream
	req := d.removeRequest(id)
	if req == nil {
		slog.Error("no pending request found for UUID", slog.String("id", id.String()))

		_ = stream.Close()

		return
	}

	select {
	case req.ch <- stream:
		return
	case <-req.ctx.Done():
		_ = stream.Close()

		return
	case <-ctx.Done():
		_ = stream.Close()

		return
	}
}

// addRequest creates a new connection request channel and associates it with a unique identifier.
// It takes a ctx of type context.Context to manage the lifecycle of the request and an id of type uuid.UUID as the identifier.
// It returns a receive-only channel of type net.Conn to deliver the connection result.
func (d *Dialer) addRequest(ctx context.Context, id uuid.UUID) <-chan net.Conn {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan net.Conn, 1)
	d.requests[id] = &connRequest{
		ctx: ctx,
		ch:  ch,
	}

	return ch
}

// removeRequest removes a connection request associated with the given ID from the Dialer's request map.
// It takes an id of type uuid.UUID which identifies the connection request.
// It returns a pointer to the connRequest if found and removes it from the map, or nil if no request exists with the given ID.
func (d *Dialer) removeRequest(id uuid.UUID) *connRequest {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch, ok := d.requests[id]
	if !ok {
		return nil
	}

	delete(d.requests, id)

	return ch
}

// WithUserPassAuth configures the dialer to use custom username-password authentication.
// It takes an auth function of type func(username, password string) bool that validates credentials.
// It returns a DialerOption which modifies the configuration of the Dialer.
func WithUserPassAuth(auth func(username, password string) bool) DialerOption {
	return func(d *Dialer) {
		d.serverOpts = append(d.serverOpts, proto.WithUserPassAuth(auth))
	}
}

// WithDialerTLSConfig configures the Dialer with TLS settings.
// It takes a tls.Config pointer and returns a DialerOption that applies the TLS configuration.
// If this option is not provided, the connection will be unencrypted.
func WithDialerTLSConfig(config *tls.Config) DialerOption {
	return func(d *Dialer) {
		d.tlsConfig = config.Clone() // Clone to prevent external modifications
	}
}
