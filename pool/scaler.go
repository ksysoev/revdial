package pool

import (
	"sync"
	"time"
)

// ScaleAction represents a scaling decision.
type ScaleAction int

const (
	// ScaleNone means no scaling action should be taken
	ScaleNone ScaleAction = iota
	// ScaleUp means a new connection should be added
	ScaleUp
	// ScaleDown means a connection should be removed
	ScaleDown
)

// AutoScaler makes decisions about when to scale the connection pool up or down.
// It uses combined metrics (stream count and latency) to determine scaling needs.
type AutoScaler struct {
	config        *Config
	lastScaleTime time.Time
	mu            sync.Mutex
}

// NewAutoScaler creates a new AutoScaler with the given configuration.
func NewAutoScaler(config *Config) *AutoScaler {
	return &AutoScaler{
		config:        config,
		lastScaleTime: time.Now(),
	}
}

// Evaluate analyzes the current metrics and returns a recommended scaling action.
// It takes pool metrics and returns a ScaleAction (ScaleUp, ScaleDown, or ScaleNone).
// The decision is based on:
// - Stream count per connection (scale up at 80% capacity, down at 20%)
// - Average latency (scale up if above threshold)
// - Cooldown period to prevent thrashing
func (s *AutoScaler) Evaluate(metrics *Metrics) ScaleAction {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cooldown check - prevent rapid scaling changes
	if time.Since(s.lastScaleTime) < s.config.ScaleCooldown {
		return ScaleNone
	}

	connCount := metrics.ConnectionCount()
	if connCount == 0 {
		return ScaleNone
	}

	activeStreams := metrics.ActiveStreams()
	avgStreamsPerConn := activeStreams / int64(connCount)

	// Scale UP conditions:
	// 1. Average streams per connection exceeds threshold (80% of max capacity)
	// 2. OR latency exceeds threshold
	// 3. AND we haven't reached max connections
	if connCount < s.config.MaxConnections {
		// Check stream capacity
		if avgStreamsPerConn > int64(s.config.ScaleUpThreshold) {
			return ScaleUp
		}

		// Check latency
		if s.config.LatencyThreshold > 0 && metrics.AvgLatency() > s.config.LatencyThreshold {
			return ScaleUp
		}
	}

	// Scale DOWN conditions:
	// 1. Average streams per connection is below threshold (20% of capacity)
	// 2. AND latency is acceptable (below half the threshold)
	// 3. AND we're above minimum connections
	if connCount > s.config.MinConnections {
		if avgStreamsPerConn < int64(s.config.ScaleDownThreshold) {
			// Also check that latency is good before scaling down
			if s.config.LatencyThreshold == 0 || metrics.AvgLatency() < s.config.LatencyThreshold/2 {
				return ScaleDown
			}
		}
	}

	return ScaleNone
}

// RecordScale records that a scaling action was taken.
// This updates the last scale time for cooldown calculations.
func (s *AutoScaler) RecordScale() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScaleTime = time.Now()
}
