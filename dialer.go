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

func (d *Dialer) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	ctx, d.cancel = context.WithCancel(ctx)

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
		d.serve(ctx)
	}()

	go func() {
		defer d.wg.Done()

		<-ctx.Done()

		_ = d.listener.Close()
	}()

	return nil
}

func (d *Dialer) Addr() string {
	return d.listener.Addr().String()
}

func (d *Dialer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cancel == nil {
		return nil
	}

	d.cancel()

	defer d.wg.Wait()

	return d.listener.Close()
}

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

func (d *Dialer) SendEvent(_ context.Context, name string, payload any) error {
	s := d.cm.GetConn()

	if s == nil || s.State() != proto.StateRegistered {
		return fmt.Errorf("no connection is available")
	}

	err := s.EmitCustomEvent(name, payload)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}

	return nil
}

func (d *Dialer) serve(ctx context.Context) {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			return
		}

		s := proto.NewServer(conn, d.serverOpts...)

		if err := s.Process(); err != nil {
			continue
		}

		switch s.State() {
		case proto.StateRegistered:
			d.cm.AddConnection(s)

		case proto.StateBound:
			id := s.ID()
			req := d.removeRequest(id)

			if req == nil {
				_ = s.Close()
				continue
			}

			select {
			case req.ch <- conn:
			case <-req.ctx.Done():
				_ = s.Close()
			}

		default:
			slog.ErrorContext(ctx, "unexpected state while handling incomming connection", slog.Any("state", s.State()))
			_ = s.Close()
		}
	}
}

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
