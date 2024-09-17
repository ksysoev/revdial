package proto

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRegister_Success(t *testing.T) {
	conn1, conn2 := net.Pipe()

	client := NewClient(conn1)
	defer client.Close()

	server := NewServer(conn2)
	defer server.Close()

	done := make(chan struct{})
	go func() {
		err := server.Process()
		assert.NoError(t, err)
		assert.Equal(t, server.State(), StateRegistered, "expected server to be in registered state")

		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Register(ctx, uuid.New())
	assert.NoError(t, err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}

func TestRegister_Failure(t *testing.T) {
	conn1, conn2 := net.Pipe()

	defer conn2.Close()

	client := NewClient(conn1)
	defer client.Close()

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 3)
		_, err := conn2.Read(buf)
		assert.NoError(t, err)

		buf = make([]byte, 2)

		_, err = conn2.Write(buf)
		assert.NoError(t, err)

		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Register(ctx, uuid.New())
	assert.Error(t, err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}

func TestSendCommand(t *testing.T) {
	expectedID := uuid.New()
	conn1, conn2 := net.Pipe()

	client := NewClient(conn1)
	defer client.Close()

	server := NewServer(conn2)
	defer server.Close()

	done := make(chan struct{})
	go func() {
		err := server.Process()
		assert.NoError(t, err)
		assert.Equal(t, server.State(), StateRegistered, "expected server to be in registered state")

		err = server.SendConnectCommand(expectedID)
		assert.NoError(t, err)

		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Register(ctx, uuid.New())
	assert.NoError(t, err)

	select {
	case cmd, ok := <-client.Commands():
		assert.True(t, ok)
		assert.Equal(t, cmd.ID, expectedID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected command to be received")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}

func TestSendCommand_Failure(t *testing.T) {
	conn1, conn2 := net.Pipe()

	client := NewClient(conn1)
	defer client.Close()

	server := NewServer(conn2)
	defer server.Close()

	done := make(chan struct{})
	go func() {
		err := server.Process()
		assert.NoError(t, err)
		assert.Equal(t, server.State(), StateRegistered, "expected server to be in registered state")

		buf := make([]byte, 2)

		_, err = conn2.Write(buf)
		assert.NoError(t, err)

		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Register(ctx, uuid.New())
	assert.NoError(t, err)

	select {
	case _, ok := <-client.Commands():
		assert.False(t, ok)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected command to be received")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}

func TestBind_Success(t *testing.T) {
	expectedID := uuid.New()
	conn1, conn2 := net.Pipe()

	client := NewClient(conn1)
	defer client.Close()

	server := NewServer(conn2)
	defer server.Close()

	done := make(chan struct{})
	go func() {
		err := server.Process()
		assert.NoError(t, err)
		assert.Equal(t, server.State(), StateBound, "expected server to be in bound state")
		assert.Equal(t, server.ID(), expectedID)

		close(done)
	}()

	err := client.Bind(expectedID)
	assert.NoError(t, err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}

func TestBind_Failure(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	client := NewClient(conn1)
	defer client.Close()

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 3)
		_, err := conn2.Read(buf)
		assert.NoError(t, err)

		buf = make([]byte, 2)

		_, err = conn2.Write(buf)
		assert.NoError(t, err)

		close(done)
	}()

	err := client.Bind(uuid.New())
	assert.Error(t, err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "expected connection to be accepted")
	}
}
