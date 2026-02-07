package mux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, uint32(1024), config.MaxStreams)
	assert.Equal(t, uint32(256*1024), config.StreamWindowSize)
	assert.Equal(t, uint32(1024*1024), config.ConnectionWindowSize)
	assert.Equal(t, 30*time.Second, config.KeepAliveInterval)
	assert.Equal(t, 60*time.Second, config.KeepAliveTimeout)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "valid config - no changes",
			config: &Config{
				MaxStreams:           2048,
				StreamWindowSize:     512 * 1024,
				ConnectionWindowSize: 2 * 1024 * 1024,
				KeepAliveInterval:    60 * time.Second,
				KeepAliveTimeout:     120 * time.Second,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, uint32(2048), c.MaxStreams)
				assert.Equal(t, uint32(512*1024), c.StreamWindowSize)
				assert.Equal(t, uint32(2*1024*1024), c.ConnectionWindowSize)
				assert.Equal(t, 60*time.Second, c.KeepAliveInterval)
				assert.Equal(t, 120*time.Second, c.KeepAliveTimeout)
			},
		},
		{
			name: "zero MaxStreams - apply default",
			config: &Config{
				MaxStreams:           0,
				StreamWindowSize:     256 * 1024,
				ConnectionWindowSize: 1024 * 1024,
				KeepAliveInterval:    30 * time.Second,
				KeepAliveTimeout:     60 * time.Second,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, uint32(1024), c.MaxStreams)
			},
		},
		{
			name: "zero StreamWindowSize - apply default",
			config: &Config{
				MaxStreams:           1024,
				StreamWindowSize:     0,
				ConnectionWindowSize: 1024 * 1024,
				KeepAliveInterval:    30 * time.Second,
				KeepAliveTimeout:     60 * time.Second,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, uint32(256*1024), c.StreamWindowSize)
			},
		},
		{
			name: "zero ConnectionWindowSize - apply default",
			config: &Config{
				MaxStreams:           1024,
				StreamWindowSize:     256 * 1024,
				ConnectionWindowSize: 0,
				KeepAliveInterval:    30 * time.Second,
				KeepAliveTimeout:     60 * time.Second,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, uint32(1024*1024), c.ConnectionWindowSize)
			},
		},
		{
			name: "zero KeepAliveInterval - apply default",
			config: &Config{
				MaxStreams:           1024,
				StreamWindowSize:     256 * 1024,
				ConnectionWindowSize: 1024 * 1024,
				KeepAliveInterval:    0,
				KeepAliveTimeout:     60 * time.Second,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, 30*time.Second, c.KeepAliveInterval)
			},
		},
		{
			name: "zero KeepAliveTimeout - apply default",
			config: &Config{
				MaxStreams:           1024,
				StreamWindowSize:     256 * 1024,
				ConnectionWindowSize: 1024 * 1024,
				KeepAliveInterval:    30 * time.Second,
				KeepAliveTimeout:     0,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, 60*time.Second, c.KeepAliveTimeout)
			},
		},
		{
			name: "all zeros - apply all defaults",
			config: &Config{
				MaxStreams:           0,
				StreamWindowSize:     0,
				ConnectionWindowSize: 0,
				KeepAliveInterval:    0,
				KeepAliveTimeout:     0,
			},
			checkFunc: func(t *testing.T, c *Config) {
				assert.Equal(t, uint32(1024), c.MaxStreams)
				assert.Equal(t, uint32(256*1024), c.StreamWindowSize)
				assert.Equal(t, uint32(1024*1024), c.ConnectionWindowSize)
				assert.Equal(t, 30*time.Second, c.KeepAliveInterval)
				assert.Equal(t, 60*time.Second, c.KeepAliveTimeout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			assert.NoError(t, err)

			if tt.checkFunc != nil {
				tt.checkFunc(t, tt.config)
			}
		})
	}
}

func TestConfig_ToYamux(t *testing.T) {
	config := &Config{
		MaxStreams:           2048,
		StreamWindowSize:     512 * 1024,
		ConnectionWindowSize: 2 * 1024 * 1024,
		KeepAliveInterval:    45 * time.Second,
		KeepAliveTimeout:     90 * time.Second,
	}

	yamuxCfg := config.ToYamux()

	assert.NotNil(t, yamuxCfg)
	assert.Equal(t, config.StreamWindowSize, yamuxCfg.MaxStreamWindowSize)
	assert.Equal(t, config.KeepAliveInterval, yamuxCfg.KeepAliveInterval)
	assert.Equal(t, config.KeepAliveTimeout, yamuxCfg.ConnectionWriteTimeout)
}

func TestConfig_ToYamux_WithDefaults(t *testing.T) {
	config := DefaultConfig()
	yamuxCfg := config.ToYamux()

	assert.NotNil(t, yamuxCfg)
	assert.Equal(t, uint32(256*1024), yamuxCfg.MaxStreamWindowSize)
	assert.Equal(t, 30*time.Second, yamuxCfg.KeepAliveInterval)
	assert.Equal(t, 60*time.Second, yamuxCfg.ConnectionWriteTimeout)
}
