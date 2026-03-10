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
	listenAddr := ":0"
	dialer := NewDialer(listenAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := dialer.Start(ctx)
	require.NoError(t, err, "Dialer should start without error")
	dialer.Addr()
	// Ensure the listener is set
	assert.NotNil(t, dialer.listener, "Listener should be initialized")

	// Ensure the listener is accepting connections
	conn, err := net.Dial("tcp", dialer.Addr())
	require.NoError(t, err, "Should be able to connect to the listener")
	conn.Close()

	// Stop the dialer and ensure it stops gracefully
	cancel()
	dialer.wg.Wait()

	// Ensure the listener is closed
	_, err = net.Dial("tcp", dialer.Addr())
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

	_, err := dialer.DialContext(ctx)
	assert.Error(t, err, "DialContext should return an error when no connection is available")
	assert.Equal(t, "no connection is available", err.Error())
}

func TestDialer_DialContext_ConnectionAvailable(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")

	defer func() {
		err := dialer.Stop()
		assert.NoError(t, err, "Dialer should stop without error")
	}()

	done := make(chan struct{})

	go func() {
		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")

		defer mockListener.Close()

		conn, err := mockListener.Accept()
		require.NoError(t, err, "Listener should accept connections")
		conn.Close()

		close(done)
	}()

	// Wait until the connection is registered in the manager before dialling.
	// Listen() returning only means the client finished its handshake; the server
	// side adds the connection to cm asynchronously, so we must poll here to
	// avoid the "no connection is available" race that appears under -race / CI.
	require.Eventually(t,
		func() bool { return dialer.cm.GetConn() != nil },
		time.Second, time.Millisecond,
		"connection should become available in the manager",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dialer.DialContext(ctx)
	require.NoError(t, err, "DialContext should not return an error when a connection is available")
	assert.NotNil(t, conn, "DialContext should return a valid connection")

	conn.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("expected connection to be accepted")
	}
}

func TestDialer_DialContext_ContextCancelled(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")

	defer func() {
		err := dialer.Stop()
		assert.NoError(t, err, "Dialer should stop without error")
	}()

	done := make(chan struct{})

	go func() {
		defer close(done)

		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")

		defer mockListener.Close()

		conn, err := mockListener.Accept()

		require.NoError(t, err, "Listener should accept connections")
		conn.Close()
	}()

	// Wait until the connection is registered before calling DialContext with a
	// cancelled context, so we exercise the cancellation path rather than the
	// "no connection available" path.
	require.Eventually(t,
		func() bool { return dialer.cm.GetConn() != nil },
		time.Second, time.Millisecond,
		"connection should become available in the manager",
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := dialer.DialContext(ctx)

	assert.Error(t, err, "DialContext should return an error when the context is cancelled")
	assert.Nil(t, conn, "DialContext should not return a connection when the context is cancelled")

	fmt.Println("err1", err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("expected connection to be accepted")
	}
}

func TestDialer_DialContext_FailedToSendConnectCommand(t *testing.T) {
	dialer := NewDialer(":0")
	err := dialer.Start(context.Background())
	require.NoError(t, err, "Dialer should start without error")

	defer func() {
		err := dialer.Stop()
		assert.NoError(t, err, "Dialer should stop without error")
	}()

	go func() {
		mockListener, err := Listen(context.Background(), dialer.Addr())
		require.NoError(t, err, "Listener should start without error")

		// Close immediately so that the control connection is gone by the time
		// DialContext sends the connect command, triggering the send error path.
		mockListener.Close()
	}()

	// Wait until the connection registers so DialContext can select it, then the
	// send will fail because the listener closed its side.
	require.Eventually(t,
		func() bool { return dialer.cm.GetConn() != nil },
		time.Second, time.Millisecond,
		"connection should become available in the manager",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dialer.DialContext(ctx)

	assert.Error(t, err, "DialContext should return an error when the connect command fails")
	assert.Nil(t, conn, "DialContext should not return a connection when the connect command fails")
}
