package revdial

import (
	"context"
	"fmt"
	"net"
	"sync"
)

type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	conn   net.Conn
}

func Listen(ctx context.Context, dialerSrv string) (*Listener, error) {
	l := &Listener{}
	l.ctx, l.cancel = context.WithCancel(ctx)

	conn, err := net.Dial("tcp", dialerSrv)
	if err != nil {
		l.cancel()
		return nil, fmt.Errorf("failed to connect to dialler server: %w", err)
	}

	// TODO: add logic to initialize connection

	l.wg.Add(1)
	go func() {
		<-l.ctx.Done()
		conn.Close()
		l.wg.Done()
	}()

	return l, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	// TODO: implement accept logic

	return nil, fmt.Error("not implemented")
}

func (l *Listener) Close() error {
	l.cancel()
	err := l.conn.Close()
	l.wg.Wait()

	return err
}
