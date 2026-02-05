package pool

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Config holds configuration for the connection pool.
type Config struct {
	// MinConnections is the minimum number of connections to maintain
	MinConnections int

	// MaxConnections is the maximum number of connections allowed
	MaxConnections int

	// ScaleUpThreshold is the target streams per connection before scaling up
	// Defaults to 800 (80% of 1024 max streams)
	ScaleUpThreshold int

	// ScaleDownThreshold is the target streams per connection before scaling down
	// Defaults to 200 (20% of 1024 max streams)
	ScaleDownThreshold int

	// LatencyThreshold is the latency threshold that triggers scale up
	LatencyThreshold time.Duration

	// ScaleCooldown is the minimum time between scale operations
	ScaleCooldown time.Duration

	// MonitorInterval is how often to evaluate scaling decisions
	MonitorInterval time.Duration
}

// DefaultConfig returns the default pool configuration.
func DefaultConfig() *Config {
	return &Config{
		MinConnections:     1,
		MaxConnections:     10,
		ScaleUpThreshold:   800, // 80% of 1024
		ScaleDownThreshold: 200, // 20% of 1024
		LatencyThreshold:   100 * time.Millisecond,
		ScaleCooldown:      30 * time.Second,
		MonitorInterval:    10 * time.Second,
	}
}

// Validate checks if the configuration is valid and applies defaults.
func (c *Config) Validate() error {
	if c.MinConnections < 1 {
		c.MinConnections = 1
	}

	if c.MaxConnections < c.MinConnections {
		c.MaxConnections = c.MinConnections
	}

	if c.ScaleUpThreshold == 0 {
		c.ScaleUpThreshold = 800
	}

	if c.ScaleDownThreshold == 0 {
		c.ScaleDownThreshold = 200
	}

	if c.ScaleDownThreshold >= c.ScaleUpThreshold {
		return fmt.Errorf("scale down threshold must be less than scale up threshold")
	}

	if c.ScaleCooldown == 0 {
		c.ScaleCooldown = 30 * time.Second
	}

	if c.MonitorInterval == 0 {
		c.MonitorInterval = 10 * time.Second
	}

	return nil
}

// Pool manages a collection of multiplexed connections.
// It provides connection selection, auto-scaling, and metrics.
type Pool struct {
	selector    Selector
	ctx         context.Context
	config      *Config
	metrics     *Metrics
	scaler      *AutoScaler
	onNewConn   func(context.Context) (*MuxConn, error)
	cancel      context.CancelFunc
	connections []*MuxConn
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

// New creates a new connection pool with the given configuration.
// It takes a Config pointer and returns a Pool pointer.
// The pool must be started with Start() before use.
func New(config *Config) *Pool {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("invalid pool config: %v", err))
	}

	return &Pool{
		config:      config,
		connections: make([]*MuxConn, 0, config.MaxConnections),
		metrics:     NewMetrics(),
		selector:    NewLeastStreamsSelector(),
		scaler:      NewAutoScaler(config),
	}
}

// Start begins the pool's auto-scaling monitoring loop.
// It takes a context for cancellation and a function to create new connections.
// The onNewConn function is called when the pool needs to add a connection.
func (p *Pool) Start(ctx context.Context, onNewConn func(context.Context) (*MuxConn, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.onNewConn = onNewConn

	p.wg.Add(1)

	go func() {
		defer p.wg.Done()
		p.monitorAndScale()
	}()
}

// AddConnection adds a new multiplexed connection to the pool.
// It takes a MuxConn pointer and adds it to the available connections.
// It returns an error if the pool is at maximum capacity.
func (p *Pool) AddConnection(conn *MuxConn) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.connections) >= p.config.MaxConnections {
		return fmt.Errorf("pool is at maximum capacity")
	}

	p.connections = append(p.connections, conn)
	p.metrics.SetConnectionCount(len(p.connections))

	slog.Info("added connection to pool",
		slog.Int("total_connections", len(p.connections)),
		slog.Int("active_streams", conn.NumStreams()))

	return nil
}

// RemoveConnection removes a connection from the pool.
// It takes a MuxConn pointer and removes it if found.
// The connection is closed before removal.
func (p *Pool) RemoveConnection(conn *MuxConn) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, c := range p.connections {
		if c != conn {
			continue
		}

		// Close the connection
		if err := c.Close(); err != nil {
			slog.Error("failed to close connection during removal", slog.Any("error", err))
		}

		// Remove from slice
		p.connections = append(p.connections[:i], p.connections[i+1:]...)
		p.metrics.SetConnectionCount(len(p.connections))

		slog.Info("removed connection from pool",
			slog.Int("total_connections", len(p.connections)))

		return nil
	}

	return fmt.Errorf("connection not found in pool")
}

// OpenStream selects a connection and opens a new stream on it.
// It takes a context for cancellation and returns a net.Conn representing the stream.
// It returns an error if no connections are available or stream creation fails.
func (p *Pool) OpenStream(ctx context.Context) (net.Conn, error) {
	p.mu.RLock()
	conn := p.selector.Select(p.connections)
	p.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("no connections available in pool")
	}

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	p.metrics.IncrementActiveStreams()

	// Wrap stream to track closure for pool metrics
	return &poolTrackedStream{
		Conn:    stream,
		onClose: p.metrics.DecrementActiveStreams,
	}, nil
}

// Connections returns a copy of the current connections slice.
func (p *Pool) Connections() []*MuxConn {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conns := make([]*MuxConn, len(p.connections))
	copy(conns, p.connections)

	return conns
}

// Metrics returns the pool's metrics.
func (p *Pool) Metrics() *Metrics {
	return p.metrics
}

// Close stops the pool and closes all connections.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}

	for _, conn := range p.connections {
		if err := conn.Close(); err != nil {
			slog.Error("failed to close connection", slog.Any("error", err))
		}
	}

	p.connections = nil
	p.metrics.SetConnectionCount(0)

	return nil
}

// monitorAndScale runs the auto-scaling monitoring loop.
func (p *Pool) monitorAndScale() {
	ticker := time.NewTicker(p.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.evaluateScaling()
		}
	}
}

// evaluateScaling checks metrics and performs scaling if needed.
func (p *Pool) evaluateScaling() {
	action := p.scaler.Evaluate(p.metrics)

	switch action {
	case ScaleUp:
		if err := p.scaleUp(); err != nil {
			slog.Error("failed to scale up", slog.Any("error", err))
		} else {
			p.scaler.RecordScale()
		}
	case ScaleDown:
		if err := p.scaleDown(); err != nil {
			slog.Error("failed to scale down", slog.Any("error", err))
		} else {
			p.scaler.RecordScale()
		}
	case ScaleNone:
		// No action needed
	}

	// Reset latency measurements for next window
	p.metrics.ResetLatency()
}

// scaleUp adds a new connection to the pool.
func (p *Pool) scaleUp() error {
	p.mu.RLock()
	currentCount := len(p.connections)
	onNewConn := p.onNewConn
	p.mu.RUnlock()

	if currentCount >= p.config.MaxConnections {
		return fmt.Errorf("already at maximum connections")
	}

	if onNewConn == nil {
		return fmt.Errorf("no connection factory configured")
	}

	slog.Info("scaling up pool", slog.Int("current_connections", currentCount))

	conn, err := onNewConn(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to create new connection: %w", err)
	}

	return p.AddConnection(conn)
}

// scaleDown removes the connection with the fewest active streams.
func (p *Pool) scaleDown() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.connections) <= p.config.MinConnections {
		return fmt.Errorf("already at minimum connections")
	}

	// Find connection with fewest streams
	var target *MuxConn
	minStreams := -1

	for _, conn := range p.connections {
		streams := conn.NumStreams()
		if minStreams == -1 || streams < minStreams {
			minStreams = streams
			target = conn
		}
	}

	if target == nil {
		return fmt.Errorf("no connection to remove")
	}

	slog.Info("scaling down pool",
		slog.Int("current_connections", len(p.connections)),
		slog.Int("target_streams", minStreams))

	// Remove by unlocking and calling RemoveConnection
	p.mu.Unlock()
	err := p.RemoveConnection(target)
	p.mu.Lock()

	return err
}

// poolTrackedStream wraps a stream to track closure for pool metrics.
type poolTrackedStream struct {
	net.Conn
	onClose func()
	once    sync.Once
}

// Close closes the stream and updates pool metrics.
func (p *poolTrackedStream) Close() error {
	p.once.Do(p.onClose)
	return p.Conn.Close()
}
