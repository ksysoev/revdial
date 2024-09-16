package revdial

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ksysoev/revdial/connmng"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDialer(t *testing.T) {
	listenAddr := "localhost:8080"
	dialer := NewDialer(listenAddr)

	assert.NotNil(t, dialer)
	assert.Equal(t, listenAddr, dialer.listen)
	assert.NotNil(t, dialer.cm)
	assert.IsType(t, &connmng.ConnManager{}, dialer.cm)
	assert.NotNil(t, dialer.requests)
	assert.Empty(t, dialer.requests)
}

func TestDialer_Start(t *testing.T) {
	listenAddr := "127.0.0.1:8080"
	dialer := NewDialer(listenAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := dialer.Start(ctx)
	require.NoError(t, err, "Dialer should start without error")

	// Ensure the listener is set
	assert.NotNil(t, dialer.listener, "Listener should be initialized")
	assert.Equal(t, listenAddr, dialer.listener.Addr().String(), "Listener address should match the provided address")

	// Ensure the listener is accepting connections
	conn, err := net.Dial("tcp", listenAddr)
	require.NoError(t, err, "Should be able to connect to the listener")
	conn.Close()

	// Stop the dialer and ensure it stops gracefully
	cancel()
	dialer.wg.Wait()

	// Ensure the listener is closed
	_, err = net.Dial("tcp", listenAddr)
	assert.Error(t, err, "Listener should be closed after stopping the dialer")
}

func TestDialer_Start_FailToListen(t *testing.T) {
	listenAddr := "invalid:address"
	dialer := NewDialer(listenAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := dialer.Start(ctx)
	assert.Error(t, err, "Dialer should return an error for invalid listen address")
}
func TestDialer_DialContext_NoConnectionAvailable(t *testing.T) {
	listenAddr := "127.0.0.1:8080"
	dialer := NewDialer(listenAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := dialer.DialContext(ctx, "")
	assert.Error(t, err, "DialContext should return an error when no connection is available")
	assert.Equal(t, "no connection is available", err.Error())
}

func TestDialer_DialContext_ConnectionAvailable(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")
	defer dialer.Stop()

	ready := make(chan struct{})
	done := make(chan struct{})

	go func() {
		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")
		defer mockListener.Close()

		close(ready)

		conn, err := mockListener.Accept()
		require.NoError(t, err, "Listener should accept connections")
		conn.Close()

		close(done)
	}()

	select {
	case <-ready:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected listener to be started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dialer.DialContext(ctx, "")
	require.NoError(t, err, "DialContext should not return an error when a connection is available")
	assert.NotNil(t, conn, "DialContext should return a valid connection")

	conn.Close()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestDialer_DialContext_ContextCancelled(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")
	defer dialer.Stop()

	ready := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")
		defer mockListener.Close()

		close(ready)

		conn, err := mockListener.Accept()

		require.NoError(t, err, "Listener should accept connections")
		conn.Close()
	}()

	select {
	case <-ready:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected listener to be started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := dialer.DialContext(ctx, "")

	assert.Error(t, err, "DialContext should return an error when the context is cancelled")
	assert.Nil(t, conn, "DialContext should not return a connection when the context is cancelled")

	fmt.Println("err1", err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestDialer_DialContext_FailedToSendConnectCommand(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")
	defer dialer.Stop()

	ready := make(chan struct{})
	go func() {
		defer close(ready)

		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")
		defer mockListener.Close()
	}()

	select {
	case <-ready:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected listener to be started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dialer.DialContext(ctx, "")

	assert.Error(t, err, "DialContext should return an error when the connect command fails")
	assert.Nil(t, conn, "DialContext should not return a connection when the connect command fails")
}
