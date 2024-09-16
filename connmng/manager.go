package connmng

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ksysoev/revdial/proto"
)

type ServerConn interface {
	ID() uuid.UUID
	Close() error
	SendConnectCommand(id uuid.UUID) error
	State() proto.State
}

type ConnManager struct {
	connections []ServerConn
	current     int
	mu          sync.RWMutex
}

// New creates and returns a new instance of ConnManager
func New() *ConnManager {
	return &ConnManager{
		connections: make([]ServerConn, 0),
		current:     0,
	}
}

// AddConnection adds a new server connection to the connection manager.
// If a connection with the same ID already exists, it closes the existing connection
// and replaces it with the new one. This method is thread-safe.
func (b *ConnManager) AddConnection(serv ServerConn) {
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

// RemoveConnection removes a connection from the ConnManager's list of connections
// based on the provided UUID. It locks the ConnManager to ensure thread safety
// during the removal process. If a connection with the specified UUID is found,
// it closes the connection and removes it from the list.
func (b *ConnManager) RemoveConnection(id uuid.UUID) {
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

// GetConn retrieves the next available ServerConn from the ConnManager's pool of connections.
// It uses a round-robin strategy to ensure that connections are distributed evenly.
// If no connections are available, it returns nil.
// This method is thread-safe.
func (b *ConnManager) GetConn() ServerConn {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.connections) == 0 {
		return nil
	}

	conn := b.connections[b.current]
	b.current = (b.current + 1) % len(b.connections)

	return conn
}

// Close closes all connections managed by ConnManager. It locks the manager
// to ensure thread safety, attempts to close each connection, and collects
// any errors encountered during the process. If any errors occur, it returns
// a combined error. Otherwise, it returns nil.
func (b *ConnManager) Close() error {
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
