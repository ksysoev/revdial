package revdial

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/ksysoev/revdial/proto"
	"github.com/stretchr/testify/assert"
)

func TestListen_FailToResolve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	listener, err := Listen(ctx, "localhost")

	var expctedErr *net.AddrError

	assert.ErrorAs(t, err, &expctedErr)
	assert.Equal(t, expctedErr.Err, "missing port in address")
	assert.Nil(t, listener)
}

func TestListen_FailToConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	listener, err := Listen(ctx, "localhost:0")

	assert.Error(t, err)
	assert.Nil(t, listener)
}

func TestListen_RegisterFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	mockListener, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)

	done := make(chan struct{})

	go func() {
		conn, err := mockListener.Accept()
		assert.NoError(t, err)
		conn.Close()
		close(done)
	}()

	listener, err := Listen(ctx, mockListener.Addr().String())

	assert.Error(t, err)
	assert.Nil(t, listener)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	mockListener, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)

	done := make(chan struct{})

	go func() {
		conn, err := mockListener.Accept()
		assert.NoError(t, err)

		s := proto.NewServer(conn)

		err = s.Process()
		assert.NoError(t, err)

		assert.Equal(t, s.State(), proto.StateRegistered)

		conn.Close()
		close(done)
	}()

	listener, err := Listen(ctx, mockListener.Addr().String())
	assert.NoError(t, err)

	assert.Equal(t, listener.Addr().String(), mockListener.Addr().String())

	defer listener.Close()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListener_Accept_Closed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	mockListener, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)

	done := make(chan struct{})

	go func() {
		conn, err := mockListener.Accept()
		assert.NoError(t, err)

		s := proto.NewServer(conn)

		err = s.Process()
		assert.NoError(t, err)

		assert.Equal(t, s.State(), proto.StateRegistered)

		conn.Close()
		close(done)
	}()

	listener, err := Listen(ctx, mockListener.Addr().String())
	assert.NoError(t, err)

	listener.Close()

	_, err = listener.Accept()

	assert.ErrorIs(t, err, ErrListenerClosed)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestWithListenerKeepAlive_NetDialer(t *testing.T) {
	l := &Listener{
		dialer: &net.Dialer{},
	}

	WithListenerKeepAlive(30 * time.Second)(l)

	d, ok := l.dialer.(*net.Dialer)
	assert.True(t, ok, "expected dialer to remain *net.Dialer")
	assert.Equal(t, 30*time.Second, d.KeepAlive)
}

func TestWithListenerKeepAlive_TLSDialer(t *testing.T) {
	l := &Listener{
		dialer: &tls.Dialer{
			NetDialer: &net.Dialer{},
			Config:    &tls.Config{},
		},
	}

	WithListenerKeepAlive(30 * time.Second)(l)

	d, ok := l.dialer.(*tls.Dialer)
	assert.True(t, ok, "expected dialer to remain *tls.Dialer")
	assert.NotNil(t, d.NetDialer)
	assert.Equal(t, 30*time.Second, d.NetDialer.KeepAlive)
}

func TestWithListenerKeepAlive_TLSDialerNilNetDialer(t *testing.T) {
	l := &Listener{
		dialer: &tls.Dialer{
			Config: &tls.Config{},
		},
	}

	WithListenerKeepAlive(30 * time.Second)(l)

	d, ok := l.dialer.(*tls.Dialer)
	assert.True(t, ok, "expected dialer to remain *tls.Dialer")
	assert.NotNil(t, d.NetDialer)
	assert.Equal(t, 30*time.Second, d.NetDialer.KeepAlive)
}

func TestWithListenerKeepAlive_KeepAliveBeforeTLS(t *testing.T) {
	l := &Listener{
		dialer: &net.Dialer{},
	}

	// Apply keepalive first, then TLS — keepalive must be preserved in tls.Dialer.NetDialer
	WithListenerKeepAlive(30 * time.Second)(l)
	WithListenerTLSConfig(&tls.Config{})(l)

	d, ok := l.dialer.(*tls.Dialer)
	assert.True(t, ok, "expected dialer to be *tls.Dialer after WithListenerTLSConfig")
	assert.NotNil(t, d.NetDialer)
	assert.Equal(t, 30*time.Second, d.NetDialer.KeepAlive)
}
