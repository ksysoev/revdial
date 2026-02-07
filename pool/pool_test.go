package pool

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 1, config.MinConnections)
	assert.Equal(t, 10, config.MaxConnections)
	assert.Equal(t, 800, config.ScaleUpThreshold)
	assert.Equal(t, 200, config.ScaleDownThreshold)
	assert.Equal(t, 100*time.Millisecond, config.LatencyThreshold)
	assert.Equal(t, 30*time.Second, config.ScaleCooldown)
	assert.Equal(t, 10*time.Second, config.MonitorInterval)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		config      *Config
		checkFunc   func(*testing.T, *Config)
		name        string
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				MinConnections:     2,
				MaxConnections:     5,
				ScaleUpThreshold:   800,
				ScaleDownThreshold: 200,
				ScaleCooldown:      30 * time.Second,
				MonitorInterval:    10 * time.Second,
			},
			expectError: false,
		},
		{
			name: "min connections < 1, should default to 1",
			config: &Config{
				MinConnections:     0,
				MaxConnections:     5,
				ScaleUpThreshold:   800,
				ScaleDownThreshold: 200,
			},
			expectError: false,
			checkFunc: func(t *testing.T, c *Config) {
				t.Helper()
				assert.Equal(t, 1, c.MinConnections)
			},
		},
		{
			name: "max < min, should adjust max",
			config: &Config{
				MinConnections: 5,
				MaxConnections: 2,
			},
			expectError: false,
			checkFunc: func(t *testing.T, c *Config) {
				t.Helper()
				assert.Equal(t, 5, c.MaxConnections)
			},
		},
		{
			name: "scale down >= scale up, should error",
			config: &Config{
				MinConnections:     1,
				MaxConnections:     10,
				ScaleUpThreshold:   800,
				ScaleDownThreshold: 800,
			},
			expectError: true,
		},
		{
			name: "zero thresholds, should apply defaults",
			config: &Config{
				MinConnections:     1,
				MaxConnections:     10,
				ScaleUpThreshold:   0,
				ScaleDownThreshold: 0,
			},
			expectError: false,
			checkFunc: func(t *testing.T, c *Config) {
				t.Helper()
				assert.Equal(t, 800, c.ScaleUpThreshold)
				assert.Equal(t, 200, c.ScaleDownThreshold)
			},
		},
		{
			name: "zero cooldown and interval, should apply defaults",
			config: &Config{
				MinConnections:     1,
				MaxConnections:     10,
				ScaleUpThreshold:   800,
				ScaleDownThreshold: 200,
				ScaleCooldown:      0,
				MonitorInterval:    0,
			},
			expectError: false,
			checkFunc: func(t *testing.T, c *Config) {
				t.Helper()
				assert.Equal(t, 30*time.Second, c.ScaleCooldown)
				assert.Equal(t, 10*time.Second, c.MonitorInterval)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, tt.config)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	assert.NotNil(t, pool)
	assert.Equal(t, config, pool.config)
	assert.NotNil(t, pool.metrics)
	assert.NotNil(t, pool.selector)
	assert.NotNil(t, pool.scaler)
	assert.Empty(t, pool.connections)
}

func TestNew_NilConfig(t *testing.T) {
	pool := New(nil)

	assert.NotNil(t, pool)
	assert.NotNil(t, pool.config)
	assert.Equal(t, DefaultConfig(), pool.config)
}

func TestNew_InvalidConfig_Panics(t *testing.T) {
	config := &Config{
		MinConnections:     1,
		MaxConnections:     10,
		ScaleUpThreshold:   200,
		ScaleDownThreshold: 800, // Invalid: down >= up
	}

	assert.Panics(t, func() {
		New(config)
	})
}

func TestPool_AddConnection(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	muxConn := createTestMuxConn(t)

	err := pool.AddConnection(muxConn)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pool.connections))
	assert.Equal(t, 1, pool.metrics.ConnectionCount())
}

func TestPool_AddConnection_AtMaxCapacity(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 2
	pool := New(config)

	// Add max connections
	for i := 0; i < 2; i++ {
		muxConn := createTestMuxConn(t)
		err := pool.AddConnection(muxConn)
		require.NoError(t, err)
	}

	// Try to add one more
	muxConn := createTestMuxConn(t)
	err := pool.AddConnection(muxConn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum capacity")
}

func TestPool_RemoveConnection(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	muxConn := createTestMuxConn(t)
	err := pool.AddConnection(muxConn)
	require.NoError(t, err)

	err = pool.RemoveConnection(muxConn)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(pool.connections))
	assert.Equal(t, 0, pool.metrics.ConnectionCount())
}

func TestPool_RemoveConnection_NotFound(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	muxConn := createTestMuxConn(t)

	err := pool.RemoveConnection(muxConn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPool_OpenStream(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	// Create both sides of yamux connection
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	yamuxCfg := yamux.DefaultConfig()

	serverSession, err := yamux.Server(serverPipe, yamuxCfg)
	require.NoError(t, err)

	defer serverSession.Close()

	clientSession, err := yamux.Client(clientPipe, yamuxCfg)
	require.NoError(t, err)

	defer clientSession.Close()

	muxConn := NewMuxConn(clientSession, clientPipe)
	err = pool.AddConnection(muxConn)
	require.NoError(t, err)

	// Accept stream on server side
	acceptDone := make(chan net.Conn, 1)

	go func() {
		stream, err := serverSession.AcceptStream()
		if err == nil {
			acceptDone <- stream
		} else {
			close(acceptDone)
		}
	}()

	ctx := context.Background()
	stream, err := pool.OpenStream(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, stream)

	// Verify metrics
	assert.Equal(t, int64(1), pool.metrics.ActiveStreams())

	// Clean up
	if stream != nil {
		stream.Close()
	}

	if serverStream := <-acceptDone; serverStream != nil {
		serverStream.Close()
	}
}

func TestPool_OpenStream_NoConnections(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	ctx := context.Background()
	stream, err := pool.OpenStream(ctx)
	assert.Error(t, err)
	assert.Nil(t, stream)
	assert.Contains(t, err.Error(), "no connections available")
}

func TestPool_Connections(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	muxConn1 := createTestMuxConn(t)
	muxConn2 := createTestMuxConn(t)

	_ = pool.AddConnection(muxConn1)
	_ = pool.AddConnection(muxConn2)

	conns := pool.Connections()
	assert.Equal(t, 2, len(conns))

	// Verify it's a copy (modifying the returned slice shouldn't affect pool)
	conns[0] = nil

	assert.NotNil(t, pool.connections[0])
}

func TestPool_Metrics(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	metrics := pool.Metrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, pool.metrics, metrics)
}

func TestPool_Close(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	muxConn1 := createTestMuxConn(t)
	muxConn2 := createTestMuxConn(t)

	_ = pool.AddConnection(muxConn1)
	_ = pool.AddConnection(muxConn2)

	err := pool.Close()
	assert.NoError(t, err)
	assert.Nil(t, pool.connections)
	assert.Equal(t, 0, pool.metrics.ConnectionCount())
}

func TestPool_Start_And_AutoScale(t *testing.T) {
	config := DefaultConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.ScaleCooldown = 0
	config.MinConnections = 1
	config.MaxConnections = 3
	pool := New(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connCount := 0

	onNewConn := func(_ context.Context) (*MuxConn, error) {
		connCount++

		return createTestMuxConn(t), nil
	}

	pool.Start(ctx, onNewConn)

	// Add initial connection
	initialConn := createTestMuxConn(t)
	err := pool.AddConnection(initialConn)
	require.NoError(t, err)

	// Give the monitor time to run
	time.Sleep(100 * time.Millisecond)

	// Close the pool
	err = pool.Close()
	assert.NoError(t, err)
}

func TestPoolTrackedStream_Close(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	callCount := 0
	onClose := func() {
		callCount++
	}

	stream := &poolTrackedStream{
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

func TestPool_ScaleUp(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 3
	pool := New(config)

	ctx := context.Background()

	connFactory := func(_ context.Context) (*MuxConn, error) {
		return createTestMuxConn(t), nil
	}

	pool.Start(ctx, connFactory)
	defer pool.Close()

	// Initially no connections
	assert.Equal(t, 0, len(pool.connections))

	// Manually trigger scale up
	err := pool.scaleUp()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pool.connections))

	// Scale up again
	err = pool.scaleUp()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pool.connections))
}

func TestPool_ScaleUp_AtMaxCapacity(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 1
	pool := New(config)

	ctx := context.Background()

	connFactory := func(_ context.Context) (*MuxConn, error) {
		return createTestMuxConn(t), nil
	}

	pool.Start(ctx, connFactory)
	defer pool.Close()

	// Add connection to reach max
	muxConn := createTestMuxConn(t)
	_ = pool.AddConnection(muxConn)

	// Try to scale up - should fail
	err := pool.scaleUp()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum connections")
}

func TestPool_ScaleUp_NoFactory(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	// Don't call Start, so no factory is set

	err := pool.scaleUp()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no connection factory")
}

func TestPool_ScaleDown(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	pool := New(config)

	// Add multiple connections
	muxConn1 := createTestMuxConn(t)
	muxConn2 := createTestMuxConn(t)
	muxConn3 := createTestMuxConn(t)

	_ = pool.AddConnection(muxConn1)
	_ = pool.AddConnection(muxConn2)
	_ = pool.AddConnection(muxConn3)

	assert.Equal(t, 3, len(pool.connections))

	// Scale down
	err := pool.scaleDown()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pool.connections))
}

func TestPool_ScaleDown_AtMinCapacity(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 2
	pool := New(config)

	// Add exactly minimum connections
	muxConn1 := createTestMuxConn(t)
	muxConn2 := createTestMuxConn(t)

	_ = pool.AddConnection(muxConn1)
	_ = pool.AddConnection(muxConn2)

	// Try to scale down - should fail
	err := pool.scaleDown()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimum connections")
}

func TestPool_EvaluateScaling(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 5
	config.ScaleUpThreshold = 800
	config.ScaleDownThreshold = 200
	pool := New(config)

	ctx := context.Background()

	connFactory := func(_ context.Context) (*MuxConn, error) {
		return createTestMuxConn(t), nil
	}

	pool.Start(ctx, connFactory)
	defer pool.Close()

	// Add a connection
	muxConn := createTestMuxConn(t)
	_ = pool.AddConnection(muxConn)

	// Evaluate with normal load - should do nothing
	pool.evaluateScaling()
	assert.Equal(t, 1, len(pool.connections))
}

// createTestMuxConn creates a MuxConn for testing
func createTestMuxConn(t *testing.T) *MuxConn {
	t.Helper()

	// Create a pipe for testing
	client, server := net.Pipe()

	// Create yamux session
	yamuxCfg := yamux.DefaultConfig()

	// Create session in server mode
	session, err := yamux.Server(server, yamuxCfg)
	if err != nil {
		t.Fatalf("failed to create yamux session: %v", err)
	}

	// Clean up when test is done
	t.Cleanup(func() {
		client.Close()
		session.Close()
	})

	return NewMuxConn(session, client)
}

func TestPool_ScaleUp_FactoryError(t *testing.T) {
	config := DefaultConfig()
	pool := New(config)

	ctx := context.Background()

	connFactory := func(_ context.Context) (*MuxConn, error) {
		return nil, assert.AnError
	}

	pool.Start(ctx, connFactory)
	defer pool.Close()

	err := pool.scaleUp()
	assert.Error(t, err)
}
