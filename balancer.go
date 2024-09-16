package revdial

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/proto"
)

// balancer manages connections and implements round-robin balancing.
type balancer struct {
	connections []*proto.Server
	current     int
	mu          sync.RWMutex
}

// newBalancer creates a new balancer.
func newBalancer() *balancer {
	return &balancer{
		connections: make([]*proto.Server, 0),
		current:     0,
	}
}

// AddConnection adds a new connection to the balancer.
func (b *balancer) AddConnection(serv *proto.Server) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, conn := range b.connections {
		if conn.ID() == serv.ID() {
			_ = conn.Close()
			b.connections[i] = serv

			return
		}
	}

	b.connections = append(b.connections, serv)
}

// RemoveConnection removes a connection from the balancer.
func (b *balancer) RemoveConnection(id uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, conn := range b.connections {
		if conn.ID() == id {
			_ = conn.Close()

			b.connections = append(b.connections[:i], b.connections[i+1:]...)

			return
		}
	}
}

// NextConnection retrieves the next connection in a round-robin manner.
func (b *balancer) Next() *proto.Server {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.connections) == 0 {
		return nil
	}

	conn := b.connections[b.current]
	b.current = (b.current + 1) % len(b.connections)

	return conn
}

func (b *balancer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	errs := make([]error, 0, len(b.connections))

	for _, conn := range b.connections {
		err := conn.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}

	b.connections = b.connections[:0]

	if len(errs) > 0 {
		return fmt.Errorf("failed to close connections: %w", errors.Join(errs...))
	}

	return nil
}
