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

type Listener struct {
	ctx        context.Context
	addr       net.Addr
	cancel     context.CancelFunc
	client     *proto.Client
	dialer     *net.Dialer
	tlsConfig  *tls.Config
	clientOpts []proto.ClientOption
}

type ListenerOption func(*Listener)

// Listen creates a new listener that listens for incoming connections.
func Listen(ctx context.Context, dialerSrv string, opts ...ListenerOption) (*Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", dialerSrv)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	l := &Listener{
		addr:   addr,
		dialer: &net.Dialer{},
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

// Accept waits for and returns the next connection to the listener.
// It returns an error if the listener is closed.
func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, ErrListenerClosed
	case cmd, ok := <-l.client.Commands():
		if !ok {
			return nil, ErrListenerClosed
		}

		switch c := cmd.(type) {
		case proto.ConnectCommand:
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

			if err := client.Bind(c.ID); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("failed to bind connection: %w", err)
			}

			return conn, nil
		default:
			return nil, fmt.Errorf("unexpected command type: %T", cmd)
		}
	}
}

// Close closes the listener and the underlying connection.
func (l *Listener) Close() error {
	l.cancel()

	return l.client.Close()
}

// Addr returns the address of remote dialer server.
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
