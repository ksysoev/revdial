package connmng

import (
	"testing"

	uuid "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	manager := New()

	assert.NotNil(t, manager, "Expected new ConnManager instance, got nil")
	assert.Empty(t, manager.connections, "Expected connections to be empty")
	assert.Equal(t, 0, manager.current, "Expected current to be 0")
}

func TestAddConnection(t *testing.T) {
	manager := New()

	// Test adding a new connection
	conn1 := NewMockServerConn(t)
	manager.AddConnection(conn1)

	conn1.EXPECT().ID().Return(uuid.New())

	assert.Len(t, manager.connections, 1, "Expected 1 connection in the manager")
	assert.Equal(t, conn1, manager.connections[0], "Expected the connection to be added")

	// Test adding another new connection
	conn2 := NewMockServerConn(t)
	duplicateUUID := uuid.New()
	conn2.EXPECT().ID().Return(duplicateUUID)
	conn2.EXPECT().Close().Return(nil)

	manager.AddConnection(conn2)

	assert.Len(t, manager.connections, 2, "Expected 2 connections in the manager")
	assert.Equal(t, conn2, manager.connections[1], "Expected the connection to be added")

	// Test adding a connection with the same ID (should replace the old one)
	conn3 := NewMockServerConn(t)
	conn3.EXPECT().ID().Return(duplicateUUID)

	manager.AddConnection(conn3)

	assert.Len(t, manager.connections, 2, "Expected 2 connections in the manager")
	assert.Equal(t, conn3, manager.connections[1], "Expected the first connection to be replaced")
}
func TestRemoveConnection(t *testing.T) {
	manager := New()

	// Test removing a connection that doesn't exist
	nonExistentID := uuid.New()
	manager.RemoveConnection(nonExistentID)

	assert.Len(t, manager.connections, 0, "Expected no connections in the manager")

	// Test removing an existing connection
	conn1 := NewMockServerConn(t)
	conn1.EXPECT().ID().Return(uuid.New())
	manager.AddConnection(conn1)

	conn2 := NewMockServerConn(t)
	conn2.EXPECT().ID().Return(uuid.New())
	conn2.EXPECT().Close().Return(nil)

	manager.AddConnection(conn2)

	assert.Len(t, manager.connections, 2, "Expected 2 connection in the manager")

	manager.RemoveConnection(conn2.ID())

	assert.Len(t, manager.connections, 1, "Expected 1 connection in the manager")

	// Test removing a connection that doesn't exist
	manager.RemoveConnection(uuid.New())

	assert.Len(t, manager.connections, 1, "Expected 1 connection in the manager")
}
func TestGetConn(t *testing.T) {
	manager := New()

	// Test getting a connection when no connections are available
	conn := manager.GetConn()
	assert.Nil(t, conn, "Expected nil when no connections are available")

	// Test getting connections in a round-robin fashion
	conn1 := NewMockServerConn(t)
	conn1ID := uuid.New()
	conn1.EXPECT().ID().Return(conn1ID)
	manager.AddConnection(conn1)

	conn2 := NewMockServerConn(t)
	conn2ID := uuid.New()
	conn2.EXPECT().ID().Return(conn2ID)
	manager.AddConnection(conn2)

	conn3 := NewMockServerConn(t)
	conn3ID := uuid.New()
	conn3.EXPECT().ID().Return(conn3ID)
	manager.AddConnection(conn3)

	assert.Len(t, manager.connections, 3, "Expected 3 connections in the manager")

	// Get connections in round-robin order
	firstConn := manager.GetConn()
	assert.Equal(t, conn1, firstConn, "Expected first connection to be conn1")

	secondConn := manager.GetConn()
	assert.Equal(t, conn2, secondConn, "Expected second connection to be conn2")

	thirdConn := manager.GetConn()
	assert.Equal(t, conn3, thirdConn, "Expected third connection to be conn3")

	// Ensure round-robin continues
	fourthConn := manager.GetConn()
	assert.Equal(t, conn1, fourthConn, "Expected fourth connection to be conn1 again")
}
func TestClose_Success(t *testing.T) {
	manager := New()

	// Test closing when no connections are available
	err := manager.Close()
	assert.NoError(t, err, "Expected no error when closing with no connections")

	// Test closing with multiple connections
	conn1 := NewMockServerConn(t)
	conn1.EXPECT().ID().Return(uuid.New())
	conn1.EXPECT().Close().Return(nil)
	manager.AddConnection(conn1)

	conn2 := NewMockServerConn(t)
	conn2.EXPECT().ID().Return(uuid.New())
	conn2.EXPECT().Close().Return(nil)
	manager.AddConnection(conn2)

	manager.AddConnection(conn1)
	manager.AddConnection(conn2)

	assert.Len(t, manager.connections, 2, "Expected 3 connections in the manager before closing")

	err = manager.Close()

	assert.NoError(t, err, "Expected no error when closing all connections")
	assert.Empty(t, manager.connections, "Expected connections to be empty after closing")
}

func TestClose_Error(t *testing.T) {
	manager := New()

	// Test closing with multiple connections, one of which returns an error
	conn1 := NewMockServerConn(t)
	conn1.EXPECT().ID().Return(uuid.New())
	conn1.EXPECT().Close().Return(nil)
	manager.AddConnection(conn1)

	conn2 := NewMockServerConn(t)
	conn2.EXPECT().ID().Return(uuid.New())
	conn2.EXPECT().Close().Return(assert.AnError)
	manager.AddConnection(conn2)

	assert.Len(t, manager.connections, 2, "Expected 3 connections in the manager before closing")

	err := manager.Close()

	assert.Error(t, err, "Expected an error when closing all connections")
	assert.ErrorIs(t, err, assert.AnError, "Expected the error to be the one returned by the connection")
	assert.Empty(t, manager.connections, "Expected connections to be empty after closing")
}
