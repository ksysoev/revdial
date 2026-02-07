package pool

import (
	"context"
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMuxConn(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxCfg := yamux.DefaultConfig()
	session, err := yamux.Server(server, yamuxCfg)
	require.NoError(t, err)

	defer session.Close()

	conn := NewMuxConn(session, client)

	assert.NotNil(t, conn)
	assert.Equal(t, session, conn.Session())
	assert.Equal(t, client, conn.Control())
	assert.NotNil(t, conn.Metrics())
	assert.False(t, conn.IsClosed())
}

func TestMuxConn_OpenStream(t *testing.T) {
	// Create a pipe for the connection
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Create yamux sessions on both sides
	yamuxCfg := yamux.DefaultConfig()

	serverSession, err := yamux.Server(serverPipe, yamuxCfg)
	require.NoError(t, err)

	defer serverSession.Close()

	clientSession, err := yamux.Client(clientPipe, yamuxCfg)
	require.NoError(t, err)

	defer clientSession.Close()

	// Create MuxConn with client session (clients open streams)
	muxConn := NewMuxConn(clientSession, clientPipe)

	// Accept stream in goroutine on server side
	acceptDone := make(chan error, 1)

	go func() {
		_, err := serverSession.AcceptStream()
		acceptDone <- err
	}()

	// Open stream
	ctx := context.Background()
	stream, err := muxConn.OpenStream(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Verify metrics updated
	assert.Equal(t, 1, muxConn.NumStreams())

	// Close stream and verify metrics
	err = stream.Close()
	assert.NoError(t, err)

	// Wait for accept to complete
	<-acceptDone
}

func TestMuxConn_OpenStream_WhenClosed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	yamuxCfg := yamux.DefaultConfig()
	session, err := yamux.Server(server, yamuxCfg)
	require.NoError(t, err)

	muxConn := NewMuxConn(session, client)

	// Close the connection
	err = muxConn.Close()
	require.NoError(t, err)

	// Try to open stream - should fail
	ctx := context.Background()
	stream, err := muxConn.OpenStream(ctx)
	assert.Error(t, err)
	assert.Nil(t, stream)
	assert.Contains(t, err.Error(), "connection is closed")
}

func TestMuxConn_AcceptStream(t *testing.T) {
	// Create a pipe for the connection
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Create yamux sessions on both sides
	yamuxCfg := yamux.DefaultConfig()

	serverSession, err := yamux.Server(serverPipe, yamuxCfg)
	require.NoError(t, err)

	defer serverSession.Close()

	clientSession, err := yamux.Client(clientPipe, yamuxCfg)
	require.NoError(t, err)

	defer clientSession.Close()

	// Create MuxConn with server session (servers accept streams)
	muxConn := NewMuxConn(serverSession, serverPipe)

	// Open stream from client in goroutine
	openDone := make(chan net.Conn, 1)

	go func() {
		stream, err := clientSession.OpenStream()
		if err == nil {
			openDone <- stream
		} else {
			close(openDone)
		}
	}()

	// Accept stream
	stream, err := muxConn.AcceptStream()
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Verify metrics updated
	assert.Equal(t, 1, muxConn.NumStreams())

	// Close stream and verify metrics
	err = stream.Close()
	assert.NoError(t, err)

	// Wait for open to complete and close client stream
	if clientStream := <-openDone; clientStream != nil {
		clientStream.Close()
	}
}

func TestMuxConn_AcceptStream_WhenClosed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	yamuxCfg := yamux.DefaultConfig()
	session, err := yamux.Server(server, yamuxCfg)
	require.NoError(t, err)

	muxConn := NewMuxConn(session, client)

	// Close the connection
	err = muxConn.Close()
	require.NoError(t, err)

	// Try to accept stream - should fail
	stream, err := muxConn.AcceptStream()
	assert.Error(t, err)
	assert.Nil(t, stream)
	assert.Contains(t, err.Error(), "connection is closed")
}

func TestMuxConn_NumStreams(t *testing.T) {
	// Create a pipe for the connection
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Create yamux sessions on both sides
	yamuxCfg := yamux.DefaultConfig()

	serverSession, err := yamux.Server(serverPipe, yamuxCfg)
	require.NoError(t, err)

	defer serverSession.Close()

	clientSession, err := yamux.Client(clientPipe, yamuxCfg)
	require.NoError(t, err)

	defer clientSession.Close()

	// Create MuxConn with client session
	muxConn := NewMuxConn(clientSession, clientPipe)

	// Initially zero streams
	assert.Equal(t, 0, muxConn.NumStreams())

	// Accept streams on server in goroutine
	acceptDone := make(chan struct{})
	acceptedStreams := make([]net.Conn, 0, 3)

	go func() {
		defer close(acceptDone)

		for i := 0; i < 3; i++ {
			stream, err := serverSession.AcceptStream()
			if err != nil {
				return
			}

			acceptedStreams = append(acceptedStreams, stream)
		}
	}()

	// Open multiple streams
	streams := make([]net.Conn, 3)

	for i := 0; i < 3; i++ {
		stream, err := muxConn.OpenStream(context.Background())
		require.NoError(t, err)

		streams[i] = stream
	}

	// Should have 3 active streams
	assert.Equal(t, 3, muxConn.NumStreams())

	// Close one stream
	err = streams[0].Close()
	require.NoError(t, err)

	// Should have 2 active streams now
	assert.Equal(t, 2, muxConn.NumStreams())

	// Close remaining streams
	for i := 1; i < 3; i++ {
		streams[i].Close()
	}

	<-acceptDone

	// Clean up accepted streams
	for _, s := range acceptedStreams {
		s.Close()
	}
}

func TestMuxConn_Close(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	yamuxCfg := yamux.DefaultConfig()
	session, err := yamux.Server(server, yamuxCfg)
	require.NoError(t, err)

	muxConn := NewMuxConn(session, client)

	assert.False(t, muxConn.IsClosed())

	// Close the connection
	err = muxConn.Close()
	assert.NoError(t, err)
	assert.True(t, muxConn.IsClosed())

	// Closing again should be idempotent
	err = muxConn.Close()
	assert.NoError(t, err)
	assert.True(t, muxConn.IsClosed())
}

func TestTrackedStream_Close(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	callCount := 0
	onClose := func() {
		callCount++
	}

	stream := &trackedStream{
		Conn:    client,
		onClose: onClose,
	}

	// Close the stream
	err := stream.Close()
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Close again - onClose should only be called once
	_ = stream.Close()
	// net.Pipe might or might not error on double close depending on timing
	assert.Equal(t, 1, callCount) // onClose called only once due to sync.Once
}
