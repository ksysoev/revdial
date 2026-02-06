package mux

import (
	"time"

	"github.com/hashicorp/yamux"
)

// Config represents the configuration for multiplexing connections.
// It contains parameters for stream management, flow control, and keepalive.
type Config struct {
	// MaxStreams is the maximum concurrent streams per connection
	MaxStreams uint32

	// StreamWindowSize is the initial stream window size in bytes
	StreamWindowSize uint32

	// ConnectionWindowSize is the connection-level window size in bytes
	ConnectionWindowSize uint32

	// KeepAliveInterval is the interval between keepalive pings
	KeepAliveInterval time.Duration

	// KeepAliveTimeout is the timeout for keepalive responses
	KeepAliveTimeout time.Duration
}

// DefaultConfig returns the default multiplexing configuration.
// It provides sensible defaults for most use cases:
// - 1024 max streams per connection
// - 256KB stream window
// - 1MB connection window
// - 30s keepalive interval
// - 60s keepalive timeout
func DefaultConfig() *Config {
	return &Config{
		MaxStreams:           1024,
		StreamWindowSize:     256 * 1024,  // 256KB
		ConnectionWindowSize: 1024 * 1024, // 1MB
		KeepAliveInterval:    30 * time.Second,
		KeepAliveTimeout:     60 * time.Second,
	}
}

// ToYamux converts the mux.Config to yamux.Config.
// It takes no parameters and returns a configured yamux.Config.
// The returned configuration is suitable for both server and client modes.
func (c *Config) ToYamux() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.MaxStreamWindowSize = c.StreamWindowSize
	cfg.KeepAliveInterval = c.KeepAliveInterval
	cfg.ConnectionWriteTimeout = c.KeepAliveTimeout

	return cfg
}

// Validate checks if the configuration is valid.
// It returns an error if any parameters are outside acceptable ranges.
func (c *Config) Validate() error {
	if c.MaxStreams == 0 {
		c.MaxStreams = DefaultConfig().MaxStreams
	}

	if c.StreamWindowSize == 0 {
		c.StreamWindowSize = DefaultConfig().StreamWindowSize
	}

	if c.ConnectionWindowSize == 0 {
		c.ConnectionWindowSize = DefaultConfig().ConnectionWindowSize
	}

	if c.KeepAliveInterval == 0 {
		c.KeepAliveInterval = DefaultConfig().KeepAliveInterval
	}

	if c.KeepAliveTimeout == 0 {
		c.KeepAliveTimeout = DefaultConfig().KeepAliveTimeout
	}

	return nil
}
