package revdial

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/proto"
)

var ErrListenerClosed = fmt.Errorf("listener closed")

type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *proto.Client
	dialer *net.Dialer
	addr   net.Addr
}

// Listen creates a new listener that listens for incoming connections.
func Listen(ctx context.Context, dialerSrv string) (*Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", dialerSrv)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	l := &Listener{
		addr:   addr,
		dialer: &net.Dialer{},
	}

	l.ctx, l.cancel = context.WithCancel(ctx)

	conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	l.client = proto.NewClient(conn)

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

		conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr.String())
		if err != nil {
			l.cancel()
			return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
		}

		client := proto.NewClient(conn)

		if err := client.Bind(cmd.ID); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to bind connection: %w", err)
		}

		return conn, nil
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
