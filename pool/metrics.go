package pool

import (
	"sync/atomic"
	"time"
)

// Metrics holds pool-level metrics for monitoring and auto-scaling decisions.
// All fields are safe for concurrent access using atomic operations.
type Metrics struct {
	activeStreams   atomic.Int64
	totalStreams    atomic.Int64
	connectionCount atomic.Int32
	bytesSent       atomic.Uint64
	bytesReceived   atomic.Uint64
	latencySum      atomic.Int64
	latencyCount    atomic.Int64
}

// NewMetrics creates and returns a new Metrics instance.
// It initializes all counters to zero.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// ActiveStreams returns the current number of active streams across all connections.
func (m *Metrics) ActiveStreams() int64 {
	return m.activeStreams.Load()
}

// IncrementActiveStreams atomically increments the active stream counter.
func (m *Metrics) IncrementActiveStreams() {
	m.activeStreams.Add(1)
	m.totalStreams.Add(1)
}

// DecrementActiveStreams atomically decrements the active stream counter.
func (m *Metrics) DecrementActiveStreams() {
	m.activeStreams.Add(-1)
}

// TotalStreams returns the total number of streams created since initialization.
func (m *Metrics) TotalStreams() int64 {
	return m.totalStreams.Load()
}

// ConnectionCount returns the current number of active connections in the pool.
func (m *Metrics) ConnectionCount() int {
	return int(m.connectionCount.Load())
}

// SetConnectionCount atomically sets the connection count.
func (m *Metrics) SetConnectionCount(count int) {
	//nolint:gosec // Connection count will never exceed int32 max
	m.connectionCount.Store(int32(count))
}

// BytesSent returns the total bytes sent across all connections.
func (m *Metrics) BytesSent() uint64 {
	return m.bytesSent.Load()
}

// AddBytesSent atomically adds to the bytes sent counter.
func (m *Metrics) AddBytesSent(n uint64) {
	m.bytesSent.Add(n)
}

// BytesReceived returns the total bytes received across all connections.
func (m *Metrics) BytesReceived() uint64 {
	return m.bytesReceived.Load()
}

// AddBytesReceived atomically adds to the bytes received counter.
func (m *Metrics) AddBytesReceived(n uint64) {
	m.bytesReceived.Add(n)
}

// RecordLatency records a latency measurement for averaging.
// It takes a duration representing the operation latency.
func (m *Metrics) RecordLatency(d time.Duration) {
	m.latencySum.Add(int64(d))
	m.latencyCount.Add(1)
}

// AvgLatency calculates and returns the average latency across all measurements.
// It returns 0 if no latency measurements have been recorded.
func (m *Metrics) AvgLatency() time.Duration {
	count := m.latencyCount.Load()
	if count == 0 {
		return 0
	}

	sum := m.latencySum.Load()

	return time.Duration(sum / count)
}

// ResetLatency resets the latency measurements.
// This is useful for windowed averaging.
func (m *Metrics) ResetLatency() {
	m.latencySum.Store(0)
	m.latencyCount.Store(0)
}

// ConnMetrics holds per-connection metrics.
type ConnMetrics struct {
	createdAt     time.Time
	totalStreams  atomic.Int64
	activeStreams atomic.Int32
}

// NewConnMetrics creates and returns a new ConnMetrics instance.
func NewConnMetrics() *ConnMetrics {
	return &ConnMetrics{
		createdAt: time.Now(),
	}
}

// ActiveStreams returns the number of active streams on this connection.
func (c *ConnMetrics) ActiveStreams() int {
	return int(c.activeStreams.Load())
}

// IncrementActiveStreams atomically increments the active stream counter.
func (c *ConnMetrics) IncrementActiveStreams() {
	c.activeStreams.Add(1)
	c.totalStreams.Add(1)
}

// DecrementActiveStreams atomically decrements the active stream counter.
func (c *ConnMetrics) DecrementActiveStreams() {
	c.activeStreams.Add(-1)
}

// TotalStreams returns the total number of streams created on this connection.
func (c *ConnMetrics) TotalStreams() int64 {
	return c.totalStreams.Load()
}

// Age returns the duration since this connection was created.
func (c *ConnMetrics) Age() time.Duration {
	return time.Since(c.createdAt)
}
