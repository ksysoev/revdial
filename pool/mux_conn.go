package pool

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
)

// MuxConn wraps a yamux.Session with metrics and management capabilities.
// It represents a single multiplexed TCP connection that can carry multiple streams.
type MuxConn struct {
	session *yamux.Session
	control net.Conn
	metrics *ConnMetrics
	mu      sync.RWMutex
	closed  bool
}

// NewMuxConn creates a new MuxConn wrapping the given yamux session.
// It takes a yamux.Session and a control stream connection.
// The control stream is used for protocol-level commands.
func NewMuxConn(session *yamux.Session, control net.Conn) *MuxConn {
	return &MuxConn{
		session: session,
		control: control,
		metrics: NewConnMetrics(),
	}
}

// OpenStream opens a new stream on this multiplexed connection.
// It takes a context for cancellation and returns a net.Conn representing the stream.
// It returns an error if the connection is closed or stream creation fails.
func (m *MuxConn) OpenStream(ctx context.Context) (net.Conn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("connection is closed")
	}

	stream, err := m.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	m.metrics.IncrementActiveStreams()

	// Wrap stream to track when it closes
	return &trackedStream{
		Conn:    stream,
		onClose: m.metrics.DecrementActiveStreams,
	}, nil
}

// AcceptStream accepts an incoming stream from the remote side.
// It returns a net.Conn representing the accepted stream.
// It returns an error if the connection is closed or acceptance fails.
func (m *MuxConn) AcceptStream() (net.Conn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("connection is closed")
	}

	stream, err := m.session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	m.metrics.IncrementActiveStreams()

	return &trackedStream{
		Conn:    stream,
		onClose: m.metrics.DecrementActiveStreams,
	}, nil
}

// NumStreams returns the current number of active streams on this connection.
func (m *MuxConn) NumStreams() int {
	return m.metrics.ActiveStreams()
}

// Control returns the control stream connection for sending protocol commands.
func (m *MuxConn) Control() net.Conn {
	return m.control
}

// Session returns the underlying yamux session.
func (m *MuxConn) Session() *yamux.Session {
	return m.session
}

// Metrics returns the metrics for this connection.
func (m *MuxConn) Metrics() *ConnMetrics {
	return m.metrics
}

// IsClosed returns true if the connection has been closed.
func (m *MuxConn) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.closed
}

// Close closes the multiplexed connection and all its streams.
// It returns an error if the closure fails.
func (m *MuxConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	if err := m.session.Close(); err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}

	return nil
}

// trackedStream wraps a yamux stream to track closure for metrics.
type trackedStream struct {
	net.Conn
	onClose func()
	once    sync.Once
}

// Close closes the stream and updates metrics.
func (t *trackedStream) Close() error {
	t.once.Do(t.onClose)
	return t.Conn.Close()
}
