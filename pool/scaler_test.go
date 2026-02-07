package pool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewAutoScaler(t *testing.T) {
	config := DefaultConfig()
	scaler := NewAutoScaler(config)

	assert.NotNil(t, scaler)
	assert.Equal(t, config, scaler.config)
	assert.True(t, time.Since(scaler.lastScaleTime) < time.Second)
}

func TestAutoScaler_Evaluate_Cooldown(t *testing.T) {
	config := DefaultConfig()
	config.ScaleCooldown = 1 * time.Second
	scaler := NewAutoScaler(config)

	// Ensure first evaluation is not affected by cooldown
	scaler.lastScaleTime = time.Now().Add(-2 * time.Second)

	metrics := NewMetrics()
	metrics.SetConnectionCount(2)
	metrics.IncrementActiveStreams() // Add some streams

	// First evaluation should work (result may vary depending on metrics and config)
	_ = scaler.Evaluate(metrics)

	// Simulate a recent scale event to trigger cooldown
	scaler.lastScaleTime = time.Now()

	// Immediately evaluate again - should return ScaleNone due to cooldown
	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action)
}

func TestAutoScaler_Evaluate_NoConnections_ScaleUp(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.ScaleCooldown = 0 // Disable cooldown for test
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(0)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleUp, action)
}

func TestAutoScaler_Evaluate_NoConnections_MinZero(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 0
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(0)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action)
}

func TestAutoScaler_Evaluate_ScaleUp_HighStreamCount(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.ScaleUpThreshold = 800
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(2)

	// Add streams to exceed threshold (800 per connection = 1600 total)
	for i := 0; i < 1700; i++ {
		metrics.IncrementActiveStreams()
	}

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleUp, action)
}

func TestAutoScaler_Evaluate_ScaleUp_HighLatency(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.LatencyThreshold = 100 * time.Millisecond
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(2)

	// Record high latency
	metrics.RecordLatency(150 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleUp, action)
}

func TestAutoScaler_Evaluate_ScaleUp_AtMaxConnections(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 2
	config.ScaleUpThreshold = 800
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(2) // Already at max

	// Add streams to exceed threshold
	for i := 0; i < 2000; i++ {
		metrics.IncrementActiveStreams()
	}

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action) // Can't scale up beyond max
}

func TestAutoScaler_Evaluate_ScaleDown_LowStreamCount(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.ScaleDownThreshold = 200
	config.LatencyThreshold = 100 * time.Millisecond
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(5)

	// Add streams below threshold (200 per connection = 1000 total)
	// Add only 500 streams (100 per connection)
	for i := 0; i < 500; i++ {
		metrics.IncrementActiveStreams()
	}

	// Record good latency
	metrics.RecordLatency(20 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleDown, action)
}

func TestAutoScaler_Evaluate_ScaleDown_AtMinConnections(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 2
	config.MaxConnections = 10
	config.ScaleDownThreshold = 200
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(2) // Already at min

	// Add streams below threshold
	for i := 0; i < 100; i++ {
		metrics.IncrementActiveStreams()
	}

	// Record good latency
	metrics.RecordLatency(20 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action) // Can't scale down beyond min
}

func TestAutoScaler_Evaluate_ScaleDown_HighLatency(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.ScaleDownThreshold = 200
	config.LatencyThreshold = 100 * time.Millisecond
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(5)

	// Add streams below threshold
	for i := 0; i < 500; i++ {
		metrics.IncrementActiveStreams()
	}

	// Record high latency (above threshold/2)
	metrics.RecordLatency(60 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action) // Don't scale down with high latency
}

func TestAutoScaler_Evaluate_ScaleNone(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.ScaleUpThreshold = 800
	config.ScaleDownThreshold = 200
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(5)

	// Add streams in the middle range (400 per connection = 2000 total)
	for i := 0; i < 2000; i++ {
		metrics.IncrementActiveStreams()
	}

	// Record good latency
	metrics.RecordLatency(20 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleNone, action)
}

func TestAutoScaler_RecordScale(t *testing.T) {
	config := DefaultConfig()
	scaler := NewAutoScaler(config)

	// Set last scale time to the past
	scaler.lastScaleTime = time.Now().Add(-1 * time.Hour)
	oldTime := scaler.lastScaleTime

	// Record a scale operation
	scaler.RecordScale()

	// Last scale time should be updated
	assert.True(t, scaler.lastScaleTime.After(oldTime))
	assert.True(t, time.Since(scaler.lastScaleTime) < time.Second)
}

func TestAutoScaler_Evaluate_ZeroLatencyThreshold(t *testing.T) {
	config := DefaultConfig()
	config.MinConnections = 1
	config.MaxConnections = 10
	config.ScaleDownThreshold = 200
	config.LatencyThreshold = 0 // Disable latency check
	scaler := NewAutoScaler(config)
	scaler.lastScaleTime = time.Time{} // Reset to avoid cooldown

	metrics := NewMetrics()
	metrics.SetConnectionCount(5)

	// Add streams below threshold
	for i := 0; i < 500; i++ {
		metrics.IncrementActiveStreams()
	}

	// Record high latency - should be ignored since threshold is 0
	metrics.RecordLatency(500 * time.Millisecond)

	action := scaler.Evaluate(metrics)
	assert.Equal(t, ScaleDown, action) // Should scale down despite high latency
}
