package revdial

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/proto"
)

type connRequest struct {
	ctx context.Context
	ch  chan net.Conn
}

type Dialer struct {
	listener net.Listener
	cancel   context.CancelFunc
	lb       *balancer
	requests map[uuid.UUID]*connRequest
	listen   string
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

func NewDialer(listen string) *Dialer {
	return &Dialer{
		listen:   listen,
		lb:       newBalancer(),
		requests: make(map[uuid.UUID]*connRequest),
	}
}

func (d *Dialer) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	ctx, d.cancel = context.WithCancel(ctx)

	l, err := net.Listen("tcp", d.listen)
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
		<-ctx.Done()
		d.listener.Close()
		d.wg.Done()
	}()

	return nil
}

func (d *Dialer) Addr() string {
	return d.listener.Addr().String()
}

func (d *Dialer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	defer d.wg.Wait()

	return d.listener.Close()
}

func (d *Dialer) DialContext(ctx context.Context, _ string) (net.Conn, error) {
	s := d.lb.Next()

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

func (d *Dialer) serve(ctx context.Context) {
	defer d.wg.Done()

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			return
		}

		s := proto.NewServer(conn)

		if err := s.Process(); err != nil {
			continue
		}

		switch s.State() {
		case proto.StateRegistered:
			d.lb.AddConnection(s)

		case proto.StateBound:
			id := s.ID()
			req := d.removeRequest(id)

			if req == nil {
				s.Close()
				continue
			}

			select {
			case req.ch <- conn:
			case <-req.ctx.Done():
				s.Close()
			}

		default:
			slog.ErrorContext(ctx, "unexpected state while handling incomming connection", slog.Any("state", s.State()))
			s.Close()
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
