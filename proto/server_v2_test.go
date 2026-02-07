package proto

import (
	"io"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)

	assert.NotNil(t, s2)
	assert.NotNil(t, s2.Server)
	assert.NotNil(t, s2.muxConfig)
	assert.False(t, s2.isV2)
	assert.Nil(t, s2.session)
}

func TestNewServerV2_WithOptions(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	customConfig := &mux.Config{
		MaxStreams:           2048,
		StreamWindowSize:     512 * 1024,
		ConnectionWindowSize: 2 * 1024 * 1024,
	}

	s2 := NewServerV2(server, nil, WithMuxConfig(customConfig))

	assert.NotNil(t, s2)
	assert.Equal(t, customConfig, s2.muxConfig)
}

func TestServerV2_IsV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)

	assert.False(t, s2.IsV2())

	// Manually set to V2 for testing
	s2.isV2 = true
	assert.True(t, s2.IsV2())
}

func TestServerV2_Session(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)

	// Initially nil
	assert.Nil(t, s2.Session())
}

func TestServerV2_AcceptStream_NotV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)

	// Should fail when not V2
	stream, err := s2.AcceptStream()
	assert.Error(t, err)
	assert.Nil(t, stream)
	assert.Contains(t, err.Error(), "not a V2 connection")
}

func TestServerV2_SendConnectCommand_V1Fallback(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	// Set up as registered V1 connection
	s2.state = StateRegistered
	s2.isV2 = false

	testID := uuid.New()

	// Client side - respond with success
	go func() {
		buf := make([]byte, 18) // version + command + UUID
		_, _ = io.ReadFull(client, buf)

		// Send success response
		_, _ = client.Write([]byte{versionV1, resSuccess})
	}()

	err := s2.SendConnectCommand(testID)
	assert.NoError(t, err)
}

func TestServerV2_SendCustomEvent(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})
	s2.state = StateRegistered

	// Client side - read the custom event and respond
	go func() {
		buf := make([]byte, 1024)
		_, _ = client.Read(buf)

		// Send success response
		_, _ = client.Write([]byte{versionV1, resSuccess})
	}()

	err := s2.SendCustomEvent("test_event", map[string]string{"key": "value"})
	assert.NoError(t, err)
}

func TestWithMuxConfig(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	customConfig := &mux.Config{
		MaxStreams: 4096,
	}

	opt := WithMuxConfig(customConfig)
	s2 := &ServerV2{
		Server:    NewServer(server),
		muxConfig: mux.DefaultConfig(),
	}

	opt(s2)

	assert.Equal(t, customConfig, s2.muxConfig)
}

func TestServerV2_Process_WrongState(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)
	s2.state = StateProcessing // Wrong state

	err := s2.Process()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected state")
}

func TestServerV2_Process_V1Register(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	testID := uuid.New()

	// Client side - simulate V1 registration
	go func() {
		// Send init request with no auth
		_, _ = client.Write([]byte{versionV1, 1, noAuth})

		// Read auth selection
		buf := make([]byte, 2)
		_, _ = io.ReadFull(client, buf)

		// Send register command with version
		_, _ = client.Write([]byte{versionV1, cmdRegister})

		// Send UUID
		_, _ = client.Write(testID[:])

		// Read response
		buf = make([]byte, 2)
		_, _ = io.ReadFull(client, buf)
	}()

	err := s2.Process()

	// Should complete without error
	assert.NoError(t, err)
	assert.Equal(t, StateRegistered, s2.State())
	assert.Equal(t, testID, s2.ID())
	assert.False(t, s2.IsV2())
}

func TestServerV2_Process_V1Bind(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	testID := uuid.New()

	// Client side - simulate V1 bind
	go func() {
		// Send init request with no auth
		_, _ = client.Write([]byte{versionV1, 1, noAuth})

		// Read auth selection
		buf := make([]byte, 2)
		_, _ = io.ReadFull(client, buf)

		// Send bind command with version
		_, _ = client.Write([]byte{versionV1, cmdBind})

		// Send UUID
		_, _ = client.Write(testID[:])

		// Read response
		buf = make([]byte, 2)
		_, _ = io.ReadFull(client, buf)
	}()

	err := s2.Process()

	// Should complete without error
	assert.NoError(t, err)
	assert.Equal(t, StateBound, s2.State())
	assert.False(t, s2.IsV2())
}

func TestServerV2_Process_UnsupportedCommand(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	// Client side - send unsupported command
	go func() {
		// Send init request with no auth
		_, _ = client.Write([]byte{versionV1, 1, noAuth})

		// Read auth selection
		buf := make([]byte, 2)
		_, _ = io.ReadFull(client, buf)

		// Send unsupported command with version
		_, _ = client.Write([]byte{versionV1, 99})
	}()

	err := s2.Process()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported command")
}

func TestServerV2_SendConnectCommandV2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})
	s2.state = StateRegistered
	s2.isV2 = true

	testID := uuid.New()

	// Client side - respond with success
	go func() {
		buf := make([]byte, 18) // version + command + UUID
		_, _ = io.ReadFull(client, buf)

		// Send success response with version
		_, _ = client.Write([]byte{versionV1, resSuccess})
	}()

	err := s2.SendConnectCommandV2(testID)
	assert.NoError(t, err)
}

func TestServerV2_SendConnectCommandV2_WrongState(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s2 := NewServerV2(server, nil)
	s2.state = StateConnected // Wrong state

	testID := uuid.New()

	err := s2.SendConnectCommandV2(testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected state")
}

func TestServerV2_SendConnectCommandV2_Failure(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	s2 := NewServerV2(server, nil)
	s2.state = StateRegistered

	testID := uuid.New()

	// Client side - respond with failure
	go func() {
		buf := make([]byte, 18) // version + command + UUID
		_, _ = io.ReadFull(client, buf)

		// Send failure response with version
		_, _ = client.Write([]byte{versionV1, resFailure})
	}()

	err := s2.SendConnectCommandV2(testID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

func TestServerV2_ProcessV2AfterAuth_ReadConfigError(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	s2 := NewServerV2(server, nil)

	// Close client immediately to cause read error
	client.Close()

	err := s2.processV2AfterAuth()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read mux config")
}

func TestServerV2_Process_InitError(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	// Close client immediately to cause init error
	client.Close()

	err := s2.Process()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to handle init")
}

func TestServerV2_Process_ReadCommandError(t *testing.T) {
	client, server := net.Pipe()

	s2 := NewServerV2(server, []ServerOption{WithNoAuth()})

	// Client side - complete init then close
	go func() {
		// Send init request
		_, _ = client.Write([]byte{versionV1, 1, noAuth})

		// Read auth selection
		buf := make([]byte, 2)
		_, _ = io.ReadFull(client, buf)

		// Close before sending command
		client.Close()
	}()

	err := s2.Process()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read command")
}

func TestServerV2_SendConnectCommandV2_WriteError(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	s2 := NewServerV2(server, nil)
	s2.state = StateRegistered

	// Close client to cause write error
	client.Close()

	testID := uuid.New()

	err := s2.SendConnectCommandV2(testID)
	require.Error(t, err)
}
