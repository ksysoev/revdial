package pool

import (
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
)

func TestNewLeastStreamsSelector(t *testing.T) {
	selector := NewLeastStreamsSelector()
	assert.NotNil(t, selector)
}

func TestLeastStreamsSelector_Select_EmptySlice(t *testing.T) {
	selector := NewLeastStreamsSelector()

	result := selector.Select(nil)
	assert.Nil(t, result)

	result = selector.Select([]*MuxConn{})
	assert.Nil(t, result)
}

func TestLeastStreamsSelector_Select_SingleConnection(t *testing.T) {
	selector := NewLeastStreamsSelector()

	// Create a mock connection
	conn := createMockMuxConn(t)
	conns := []*MuxConn{conn}

	result := selector.Select(conns)
	assert.Equal(t, conn, result)
}

func TestLeastStreamsSelector_Select_MultipleConnections(t *testing.T) {
	selector := NewLeastStreamsSelector()

	// Create mock connections with different stream counts
	conn1 := createMockMuxConn(t)
	conn2 := createMockMuxConn(t)
	conn3 := createMockMuxConn(t)

	// Manually set different stream counts in metrics
	conn1.metrics.IncrementActiveStreams()
	conn1.metrics.IncrementActiveStreams()
	conn1.metrics.IncrementActiveStreams()

	conn2.metrics.IncrementActiveStreams()

	// conn3 has zero streams

	conns := []*MuxConn{conn1, conn2, conn3}

	// Should select conn3 (fewest streams)
	result := selector.Select(conns)
	assert.Equal(t, conn3, result)
	assert.Equal(t, 0, result.NumStreams())

	// Add more streams to conn3
	conn3.metrics.IncrementActiveStreams()
	conn3.metrics.IncrementActiveStreams()

	// conn1: 3 streams, conn2: 1 stream, conn3: 2 streams
	result = selector.Select(conns)
	assert.Equal(t, conn2, result)
	assert.Equal(t, 1, result.NumStreams())
}

// createMockMuxConn creates a mock MuxConn for testing
func createMockMuxConn(t *testing.T) *MuxConn {
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

	// Clean up the client side
	t.Cleanup(func() {
		client.Close()
		session.Close()
	})

	return NewMuxConn(session, client)
}
