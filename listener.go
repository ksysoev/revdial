package revdial

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/ksysoev/revdial/proto"
)

type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *proto.Client
	dialer net.Dialer
	addr   string
	wg     sync.WaitGroup
}

func Listen(ctx context.Context, dialerSrv string) (*Listener, error) {
	l := &Listener{addr: dialerSrv}
	l.ctx, l.cancel = context.WithCancel(ctx)

	conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr)
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	l.wg.Add(1)

	go func() {
		defer l.wg.Done()
		<-l.ctx.Done()
		conn.Close()
	}()

	l.client = proto.NewClient(conn)

	err = l.client.Register(l.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %w", err)
	}

	return l, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, fmt.Errorf("listener closed")
	case cmd, ok := <-l.client.Commands():
		if !ok {
			return nil, fmt.Errorf("connection closed")
		}

		conn, err := l.dialer.DialContext(l.ctx, "tcp", l.addr)
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

func (l *Listener) Close() error {
	l.cancel()
	err := l.client.Close()
	l.wg.Wait()

	return err
}

func (l *Listener) Addr() net.Addr {
	// TODO: implement this
	panic("not implemented")
}
