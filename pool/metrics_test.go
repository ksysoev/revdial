package pool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()
	assert.NotNil(t, m)
	assert.Equal(t, int64(0), m.ActiveStreams())
	assert.Equal(t, int64(0), m.TotalStreams())
	assert.Equal(t, 0, m.ConnectionCount())
	assert.Equal(t, uint64(0), m.BytesSent())
	assert.Equal(t, uint64(0), m.BytesReceived())
	assert.Equal(t, time.Duration(0), m.AvgLatency())
}

func TestMetrics_ActiveStreams(t *testing.T) {
	m := NewMetrics()

	// Test increment
	m.IncrementActiveStreams()
	assert.Equal(t, int64(1), m.ActiveStreams())
	assert.Equal(t, int64(1), m.TotalStreams())

	m.IncrementActiveStreams()
	assert.Equal(t, int64(2), m.ActiveStreams())
	assert.Equal(t, int64(2), m.TotalStreams())

	// Test decrement
	m.DecrementActiveStreams()
	assert.Equal(t, int64(1), m.ActiveStreams())
	assert.Equal(t, int64(2), m.TotalStreams()) // Total should not change on decrement
}

func TestMetrics_ConnectionCount(t *testing.T) {
	m := NewMetrics()

	m.SetConnectionCount(5)
	assert.Equal(t, 5, m.ConnectionCount())

	m.SetConnectionCount(10)
	assert.Equal(t, 10, m.ConnectionCount())

	m.SetConnectionCount(0)
	assert.Equal(t, 0, m.ConnectionCount())
}

func TestMetrics_BytesSent(t *testing.T) {
	m := NewMetrics()

	m.AddBytesSent(100)
	assert.Equal(t, uint64(100), m.BytesSent())

	m.AddBytesSent(50)
	assert.Equal(t, uint64(150), m.BytesSent())
}

func TestMetrics_BytesReceived(t *testing.T) {
	m := NewMetrics()

	m.AddBytesReceived(200)
	assert.Equal(t, uint64(200), m.BytesReceived())

	m.AddBytesReceived(300)
	assert.Equal(t, uint64(500), m.BytesReceived())
}

func TestMetrics_Latency(t *testing.T) {
	m := NewMetrics()

	// Initially zero
	assert.Equal(t, time.Duration(0), m.AvgLatency())

	// Record some latencies
	m.RecordLatency(100 * time.Millisecond)
	assert.Equal(t, 100*time.Millisecond, m.AvgLatency())

	m.RecordLatency(200 * time.Millisecond)
	assert.Equal(t, 150*time.Millisecond, m.AvgLatency())

	m.RecordLatency(300 * time.Millisecond)
	assert.Equal(t, 200*time.Millisecond, m.AvgLatency())

	// Reset
	m.ResetLatency()
	assert.Equal(t, time.Duration(0), m.AvgLatency())

	// Record new latencies after reset
	m.RecordLatency(50 * time.Millisecond)
	assert.Equal(t, 50*time.Millisecond, m.AvgLatency())
}

func TestNewConnMetrics(t *testing.T) {
	cm := NewConnMetrics()
	assert.NotNil(t, cm)
	assert.Equal(t, 0, cm.ActiveStreams())
	assert.Equal(t, int64(0), cm.TotalStreams())
	assert.True(t, cm.Age() < time.Second) // Should be very fresh
}

func TestConnMetrics_ActiveStreams(t *testing.T) {
	cm := NewConnMetrics()

	// Test increment
	cm.IncrementActiveStreams()
	assert.Equal(t, 1, cm.ActiveStreams())
	assert.Equal(t, int64(1), cm.TotalStreams())

	cm.IncrementActiveStreams()
	assert.Equal(t, 2, cm.ActiveStreams())
	assert.Equal(t, int64(2), cm.TotalStreams())

	// Test decrement
	cm.DecrementActiveStreams()
	assert.Equal(t, 1, cm.ActiveStreams())
	assert.Equal(t, int64(2), cm.TotalStreams()) // Total should not change
}

func TestConnMetrics_Age(t *testing.T) {
	cm := NewConnMetrics()

	// Age should be near zero
	age1 := cm.Age()
	assert.True(t, age1 >= 0)
	assert.True(t, age1 < 100*time.Millisecond)

	// Wait a bit and check age increased
	time.Sleep(10 * time.Millisecond)

	age2 := cm.Age()
	assert.True(t, age2 > age1)
}
