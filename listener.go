package revdial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/mux"
	"github.com/ksysoev/revdial/pool"
	"github.com/ksysoev/revdial/proto"
)

var ErrListenerClosed = fmt.Errorf("listener closed")

type EventHandler func(event Event)

type Event interface {
	ParsePayload(v any) error
}

type dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type Listener struct {
	ctx           context.Context
	addr          net.Addr
	dialer        dialer
	cancel        context.CancelFunc
	client        *proto.ClientV2
	eventHandlers map[proto.CommandType]EventHandler
	pool          *pool.Pool
	muxConfig     *mux.Config
	clientOpts    []proto.ClientOption
	clientV2Opts  []proto.ClientV2Option
	useV2         bool
}

type ListenerOption func(*Listener)

// Listen establishes a listener connection to a remote dialer server.
// It takes a context (ctx), a dialer server address (dialerSrv), and optional configuration (opts).
// It returns a pointer to a Listener instance or an error.
// It returns an error if the address resolution, connection, TLS handshake, or client registration fails.
// The listener now supports both V1 and V2 protocols with automatic detection.
func Listen(ctx context.Context, dialerSrv string, opts ...ListenerOption) (*Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", dialerSrv)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	l := &Listener{
		addr:          addr,
		dialer:        &net.Dialer{},
		eventHandlers: make(map[proto.CommandType]EventHandler),
		useV2:         false, // V2 disabled by default for backward compatibility
	}

	for _, opt := range opts {
		opt(l)
	}

	l.ctx, l.cancel = context.WithCancel(ctx)

	// If V2 is disabled, force V1 mode
	if !l.useV2 {
		l.clientOpts = append(l.clientOpts, proto.WithDisableV2Fallback())
	}

	conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	// Use ClientV2 which supports both protocols
	l.client = proto.NewClientV2(conn, l.clientOpts, l.clientV2Opts...)

	err = l.client.Register(l.ctx, uuid.New())
	if err != nil {
		l.cancel()

		_ = conn.Close()

		return nil, fmt.Errorf("failed to register client: %w", err)
	}

	// If V2 succeeded, initialize pool
	if l.client.IsV2() && l.pool != nil {
		// Add initial connection to pool (control stream is managed by client)
		session := l.client.Session()
		if session == nil {
			l.cancel()
			return nil, fmt.Errorf("V2 client has no yamux session")
		}

		muxConn := pool.NewMuxConn(session, nil)
		if err := l.pool.AddConnection(muxConn); err != nil {
			return nil, fmt.Errorf("failed to add connection to pool: %w", err)
		}

		// Start pool monitoring
		l.pool.Start(l.ctx, l.createPoolConnection)
	}

	return l, nil
}

// createPoolConnection is called by the pool to create new connections.
func (l *Listener) createPoolConnection(ctx context.Context) (*pool.MuxConn, error) {
	conn, err := l.dialer.DialContext(ctx, "tcp", l.addr.String())
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := proto.NewClientV2(conn, l.clientOpts, l.clientV2Opts...)
	if err := client.Register(ctx, uuid.New()); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to register: %w", err)
	}

	if !client.IsV2() {
		_ = client.Close()
		return nil, fmt.Errorf("server does not support V2")
	}

	session := client.Session()
	if session == nil {
		_ = client.Close()
		return nil, fmt.Errorf("V2 client has no yamux session")
	}

	return pool.NewMuxConn(session, nil), nil
}

// Accept waits for and accepts a connection request from the client.
// It returns a net.Conn representing the established connection or an error.
// It returns an error if the listener is closed, fails to parse a command, fails to connect, or encounters a binding error.
// In V2 mode, it opens a new stream on the existing multiplexed connection instead of creating a new TCP connection.
func (l *Listener) Accept() (net.Conn, error) {
	for {
		select {
		case <-l.ctx.Done():
			return nil, ErrListenerClosed
		case cmd, ok := <-l.client.Commands():
			if !ok {
				return nil, ErrListenerClosed
			}

			switch cmd.Type() {
			case proto.ConnectCommandType:
				var id uuid.UUID

				err := cmd.ParsePayload(&id)
				if err != nil {
					return nil, fmt.Errorf("failed to parse command payload: %w", err)
				}

				// Use V2 stream if available
				if l.client.IsV2() {
					return l.acceptV2Stream(id)
				}

				// V1 fallback - create new TCP connection
				return l.acceptV1Connection(id)
			default:
				if handler, ok := l.eventHandlers[cmd.Type()]; ok {
					handler(cmd)
					continue
				}

				return nil, fmt.Errorf("unexpected command type: %T", cmd)
			}
		}
	}
}

// acceptV2Stream opens a new stream on the multiplexed connection for V2 protocol.
func (l *Listener) acceptV2Stream(id uuid.UUID) (net.Conn, error) {
	// Open a stream from the pool if available, otherwise from the primary client
	if l.pool != nil {
		stream, err := l.pool.OpenStream(l.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to open stream from pool: %w", err)
		}

		// Send bind command on the stream
		if err := l.bindStream(stream, id); err != nil {
			_ = stream.Close()
			return nil, err
		}

		return stream, nil
	}

	// Use primary client session directly
	session := l.client.Session()
	if session == nil {
		return nil, fmt.Errorf("no yamux session available")
	}

	stream, err := session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	// Send bind command on the stream
	if err := l.bindStream(stream, id); err != nil {
		_ = stream.Close()
		return nil, err
	}

	return stream, nil
}

// acceptV1Connection creates a new TCP connection for V1 protocol.
func (l *Listener) acceptV1Connection(id uuid.UUID) (net.Conn, error) {
	conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	client := proto.NewClient(conn, l.clientOpts...)

	if err := client.Bind(l.ctx, id); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to bind connection: %w", err)
	}

	return conn, nil
}

// bindStream sends a bind command on the given stream.
// V2 streams use V1 format for compatibility with existing protocol parsing.
func (l *Listener) bindStream(stream net.Conn, id uuid.UUID) error {
	req := make([]byte, 18)
	req[0] = proto.VersionV1() // V2 streams use V1 format for compatibility
	req[1] = proto.CmdBind()
	copy(req[2:], id[:])

	if _, err := stream.Write(req); err != nil {
		return fmt.Errorf("failed to write bind command: %w", err)
	}

	// Read response
	resp := make([]byte, 2)
	if _, err := stream.Read(resp); err != nil {
		return fmt.Errorf("failed to read bind response: %w", err)
	}

	if resp[0] != 1 || resp[1] != proto.ResSuccess() {
		return fmt.Errorf("bind failed: version=%d, result=%d", resp[0], resp[1])
	}

	return nil
}

// Close terminates the Listener and releases associated resources.
// It returns an error if the underlying client fails to close properly.
func (l *Listener) Close() error {
	l.cancel()

	if l.pool != nil {
		_ = l.pool.Close()
	}

	return l.client.Close()
}

// Addr returns the network address the Listener is bound to.
// It takes no parameters.
// It returns a net.Addr representing the address of the Listener.
func (l *Listener) Addr() net.Addr {
	return l.addr
}

// WithUserPass configures a Listener with username and password authentication.
// It takes a username and password, both of type string.
// It returns a ListenerOption that applies the authentication to the Listener.
// It returns an error if the username or password exceeds 255 characters or if the underlying configuration fails.
func WithUserPass(username, password string) (ListenerOption, error) {
	opt, err := proto.WithUserPass(username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to create user pass auth option: %w", err)
	}

	return func(l *Listener) {
		l.clientOpts = append(l.clientOpts, opt)
	}, nil
}

// WithListenerTLSConfig configures a Listener with TLS settings.
// It takes a tls.Config pointer and returns a ListenerOption that applies the TLS configuration.
// If this option is not provided, the connection will be unencrypted.
func WithListenerTLSConfig(config *tls.Config) ListenerOption {
	return func(l *Listener) {
		d, _ := l.dialer.(*net.Dialer)
		l.dialer = &tls.Dialer{
			NetDialer: d,
			Config:    config.Clone(),
		}
	}
}

// WithEventHandler registers an event handler for a specific event type.
// It takes eventName of type string and handler of type func(event Event).
// It returns a ListenerOption to configure the listener and an error if the event name is invalid or reserved.
func WithEventHandler(eventName string, handler func(event Event)) (ListenerOption, error) {
	cmdType, err := proto.NewCommandType(eventName)
	if err != nil {
		return nil, fmt.Errorf("failed to create event handler: %w", err)
	}

	return func(l *Listener) {
		l.eventHandlers[cmdType] = handler
	}, nil
}

// WithPoolConfig configures the connection pool for V2 multiplexing.
// It takes a pool.Config pointer and returns a ListenerOption.
// The pool is only used when V2 protocol is successfully negotiated.
func WithPoolConfig(config *pool.Config) ListenerOption {
	return func(l *Listener) {
		l.pool = pool.New(config)
	}
}

// WithMuxConfig configures multiplexing parameters for V2 connections.
// It takes a mux.Config pointer and returns a ListenerOption.
func WithMuxConfig(config *mux.Config) ListenerOption {
	return func(l *Listener) {
		l.muxConfig = config
		l.clientV2Opts = append(l.clientV2Opts, proto.WithMuxConfigClient(config))
	}
}

// WithDisableV2 disables V2 protocol and forces V1 mode.
// It returns a ListenerOption that configures the listener to only use V1.
func WithDisableV2() ListenerOption {
	return func(l *Listener) {
		l.useV2 = false
	}
}

// WithEnableV2 enables V2 protocol with multiplexing support.
// It returns a ListenerOption that configures the listener to use V2 with fallback to V1.
// When enabled, the listener will attempt V2 protocol first and fallback to V1 if unsupported.
func WithEnableV2() ListenerOption {
	return func(l *Listener) {
		l.useV2 = true
	}
}
