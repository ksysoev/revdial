package revdial

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/ksysoev/revdial/proto"
)

type Dialer struct {
	listen   string
	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.Mutex
	cancel   context.CancelFunc
	server   *proto.Server
}

func NewDialer(listen string) *Dialer {
	return &Dialer{
		listen: listen,
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

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.serve(ctx)
	}()

	d.wg.Add(1)
	go func() {
		<-ctx.Done()
		d.listener.Close()
		d.wg.Done()
	}()

	return nil
}

func (d *Dialer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	defer d.wg.Wait()

	return d.listener.Close()
}

func (d *Dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	// TODO: implement dial logic

	return nil, fmt.Errorf("not implemented")
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
			// TODO: What to do with registered server?
			d.mu.Lock()
			oldServer := d.server
			d.server = s
			d.mu.Unlock()

			if oldServer != nil {
				server.Close()
			}
		case proto.StateBound:
			//TODO: handle bound state
		default:
			slog.ErrorContext(ctx, "unexpected state while handling incomming connection", slog.Any("state", s.State()))
			s.Close()
		}
	}

}
