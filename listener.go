package revdial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/proto"
)

var ErrListenerClosed = fmt.Errorf("listener closed")

type EventHandler func(event Event)

type Event interface {
	ParsePayload(v any) error
}
type Listener struct {
	ctx           context.Context
	addr          net.Addr
	cancel        context.CancelFunc
	client        *proto.Client
	dialer        *net.Dialer
	tlsConfig     *tls.Config
	eventHandlers map[proto.CommandType]EventHandler
	clientOpts    []proto.ClientOption
}

type ListenerOption func(*Listener)

// Listen establishes a listener connection to a remote dialer server.
// It takes a context (ctx), a dialer server address (dialerSrv), and optional configuration (opts).
// It returns a pointer to a Listener instance or an error.
// It returns an error if the address resolution, connection, TLS handshake, or client registration fails.
func Listen(ctx context.Context, dialerSrv string, opts ...ListenerOption) (*Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", dialerSrv)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	l := &Listener{
		addr:          addr,
		dialer:        &net.Dialer{},
		eventHandlers: make(map[proto.CommandType]EventHandler),
	}

	for _, opt := range opts {
		opt(l)
	}

	l.ctx, l.cancel = context.WithCancel(ctx)

	conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	if l.tlsConfig != nil {
		tlsConn := tls.Client(conn, l.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()

			l.cancel()

			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}

		conn = tlsConn
	}

	l.client = proto.NewClient(conn, l.clientOpts...)

	err = l.client.Register(l.ctx, uuid.New())
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %w", err)
	}

	return l, nil
}

// Accept waits for and accepts a connection request from the client.
// It returns a net.Conn representing the established connection or an error.
// It returns an error if the listener is closed, fails to parse a command, fails to connect, or encounters a binding error.
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

				conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
				if err != nil {
					l.cancel()
					return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
				}

				if l.tlsConfig != nil {
					tlsConn := tls.Client(conn, l.tlsConfig)
					if err := tlsConn.Handshake(); err != nil {
						_ = conn.Close()
						return nil, fmt.Errorf("TLS handshake failed: %w", err)
					}

					conn = tlsConn
				}

				client := proto.NewClient(conn, l.clientOpts...)

				if err := client.Bind(id); err != nil {
					_ = conn.Close()
					return nil, fmt.Errorf("failed to bind connection: %w", err)
				}

				return conn, nil
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

// Close terminates the Listener and releases associated resources.
// It returns an error if the underlying client fails to close properly.
func (l *Listener) Close() error {
	l.cancel()

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
		l.tlsConfig = config.Clone() // Clone to prevent external modifications
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
