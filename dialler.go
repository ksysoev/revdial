package revdial

import (
	"context"
	"fmt"
	"net"
	"sync"
)

type Dialer struct {
	listen   string
	listener net.Listener
	wg       sync.WaitGroup
}

func NewDialer(listen string) *Dialer {
	return &Dialer{
		listen: listen,
	}
}

func (d *Dialer) Start(ctx context.Context) error {
	l, err := net.Listen("tcp", d.listen)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	d.listener = l

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		<-ctx.Done()
		l.Close()
	}()

	return nil
}

func (d *Dialer) Stop() error {
	defer d.wg.Wait()

	return d.listener.Close()
}

func (d *Dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	// TODO: implement dial logic

	return nil, fmt.Errorf("not implemented")
}
