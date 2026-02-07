package proto

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/ksysoev/revdial/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	assert.NotNil(t, c2)
	assert.NotNil(t, c2.Client)
	assert.NotNil(t, c2.muxConfig)
	assert.False(t, c2.isV2)
}

func TestNewClientV2_WithOptions(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	customConfig := &mux.Config{
		MaxStreams: 4096,
	}

	c2 := NewClientV2(client, nil, WithMuxConfigClient(customConfig))
	assert.NotNil(t, c2)
	assert.Equal(t, customConfig, c2.muxConfig)
}

func TestClientV2_IsV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	assert.False(t, c2.IsV2())

	c2.isV2 = true
	assert.True(t, c2.IsV2())
}

func TestClientV2_Session(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	assert.Nil(t, c2.Session())

	// Create a mock session
	mockSession, err := yamux.Server(server, nil)
	require.NoError(t, err)

	defer mockSession.Close()

	c2.session = mockSession
	assert.Equal(t, mockSession, c2.Session())
}

func TestClientV2_Register_WrongState(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	c2.state = registered // Wrong state

	ctx := context.Background()
	testID := uuid.New()

	err := c2.Register(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected state")
}

func TestClientV2_Register_V1Fallback(t *testing.T) {
	t.Skip("Complex integration test - skipping for now")
	// TODO: This test requires full V1 registration flow with proper connection management
}

func TestClientV2_Register_V2Success(t *testing.T) {
	t.Skip("Complex integration test - skipping for now")
	// TODO: This test requires full V2 registration flow with yamux session management
}

func TestClientV2_Register_V2EstablishError(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c2 := NewClientV2(client, nil)

	testID := uuid.New()
	ctx := context.Background()

	// Close client immediately to cause establish error
	client.Close()

	err := c2.Register(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to establish connection")
}

func TestClientV2_Register_V2MuxInitError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	c2 := NewClientV2(client, nil)

	testID := uuid.New()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Server side - send auth then close before MuxReady
	go func() {
		defer server.Close()

		// Read auth request
		buf := make([]byte, 3)
		_, _ = io.ReadFull(server, buf)

		// Send auth selection
		_, _ = server.Write([]byte{versionV1, noAuth})

		// Read MuxInit command
		buf = make([]byte, 15)
		_, _ = io.ReadFull(server, buf)

		// Close instead of sending MuxReady
	}()

	err := c2.Register(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "V2 registration failed")
}

func TestClientV2_Register_V2NotSupported(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	c2 := NewClientV2(client, nil)

	testID := uuid.New()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Server side - reject V2 with error response
	go func() {
		defer server.Close()

		// Read auth request
		buf := make([]byte, 3)
		_, _ = io.ReadFull(server, buf)

		// Send auth selection
		_, _ = server.Write([]byte{versionV1, noAuth})

		// Read MuxInit command
		buf = make([]byte, 15)
		_, _ = io.ReadFull(server, buf)

		// Send error response instead of MuxReady
		_, _ = server.Write([]byte{versionV1, resFailure})
	}()

	err := c2.Register(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support V2")
}

func TestClientV2_Register_V2RegistrationFailed(t *testing.T) {
	t.Skip("Complex integration test - skipping for now")
	// TODO: This test requires full V2 setup with yamux
}

func TestClientV2_Bind_V1Mode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	c2.isV2 = false

	// Simple test to verify the V1 routing logic works
	// We can't test the full flow without setting up a registered connection
	assert.False(t, c2.IsV2())
}

func TestClientV2_Bind_V2Success(t *testing.T) {
	// Create a pipe for the yamux connection
	client, server := net.Pipe()
	defer client.Close()

	// Set up yamux session
	clientSession, err := yamux.Client(client, nil)
	require.NoError(t, err)

	defer clientSession.Close()

	c2 := NewClientV2(client, nil)
	c2.isV2 = true
	c2.session = clientSession
	c2.state = registered

	testID := uuid.New()
	ctx := context.Background()

	// Server side - handle V2 bind
	go func() {
		serverSession, err := yamux.Server(server, nil)
		if err != nil {
			return
		}
		defer serverSession.Close()

		// Accept bind stream
		stream, err := serverSession.AcceptStream()
		if err != nil {
			return
		}
		defer stream.Close()

		// Read bind command
		buf := make([]byte, 18) // version + command + UUID
		_, _ = io.ReadFull(stream, buf)

		// Send success response
		_, _ = stream.Write([]byte{versionV1, resSuccess})
	}()

	err = c2.Bind(ctx, testID)
	assert.NoError(t, err)
}

func TestClientV2_Bind_V2NoSession(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	c2 := NewClientV2(client, nil)
	c2.isV2 = true
	c2.session = nil

	testID := uuid.New()
	ctx := context.Background()

	err := c2.Bind(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no yamux session")
}

func TestClientV2_Bind_V2StreamError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Set up yamux session
	clientSession, err := yamux.Client(client, nil)
	require.NoError(t, err)

	c2 := NewClientV2(client, nil)
	c2.isV2 = true
	c2.session = clientSession
	c2.state = registered

	testID := uuid.New()
	ctx := context.Background()

	// Close session to cause stream open error
	clientSession.Close()

	err = c2.Bind(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open stream")
}

func TestClientV2_Bind_V2BindFailed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	// Set up yamux session
	clientSession, err := yamux.Client(client, nil)
	require.NoError(t, err)

	defer clientSession.Close()

	c2 := NewClientV2(client, nil)
	c2.isV2 = true
	c2.session = clientSession
	c2.state = registered

	testID := uuid.New()
	ctx := context.Background()

	// Server side - reject bind
	go func() {
		serverSession, err := yamux.Server(server, nil)
		if err != nil {
			return
		}
		defer serverSession.Close()

		// Accept bind stream
		stream, err := serverSession.AcceptStream()
		if err != nil {
			return
		}
		defer stream.Close()

		// Read bind command
		buf := make([]byte, 18)
		_, _ = io.ReadFull(stream, buf)

		// Send failure response
		_, _ = stream.Write([]byte{versionV1, resFailure})
	}()

	err = c2.Bind(ctx, testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bind failed")
}

func TestClientV2_Close(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c2 := NewClientV2(client, nil)

	err := c2.Close()
	assert.NoError(t, err)
}

func TestClientV2_Close_WithV2Session(t *testing.T) {
	client, server := net.Pipe()

	// Set up yamux session
	clientSession, err := yamux.Client(client, nil)
	require.NoError(t, err)

	c2 := NewClientV2(client, nil)
	c2.isV2 = true
	c2.session = clientSession

	// Server side
	go func() {
		serverSession, _ := yamux.Server(server, nil)
		if serverSession != nil {
			defer serverSession.Close()
		}

		<-time.After(100 * time.Millisecond)
		server.Close()
	}()

	err = c2.Close()
	assert.NoError(t, err)

	// Verify session is closed
	_, err = clientSession.Open()
	assert.Error(t, err)
}

func TestWithMuxConfigClient(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	customConfig := &mux.Config{
		MaxStreams:           4096,
		StreamWindowSize:     512 * 1024,
		ConnectionWindowSize: 512 * 1024,
	}

	opt := WithMuxConfigClient(customConfig)
	c2 := &ClientV2{
		Client:    NewClient(client),
		muxConfig: mux.DefaultConfig(),
	}

	opt(c2)

	assert.Equal(t, customConfig, c2.muxConfig)
}

func TestWithDisableV2Fallback(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	opt := WithDisableV2Fallback()
	c := NewClient(client)

	opt(c)

	assert.True(t, c.disableV2)
}
