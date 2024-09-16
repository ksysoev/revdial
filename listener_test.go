package revdial

import (
	"context"
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
