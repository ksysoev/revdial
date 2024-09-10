package revdial

import (
	"context"
	"testing"
	"time"
)

func TestListenerDialer(t *testing.T) {
	// Create a new dialer
	dialer := NewDialer(":0")

	if err := dialer.Start(context.Background()); err != nil {
		t.Fatalf("failed to start dialer: %v", err)
	}

	defer func() {
		err := dialer.Stop()
		if err != nil {
			t.Errorf("failed to stop dialer: %v", err)
		}
	}()

	addr := dialer.listener.Addr().String()

	// Create a new listener
	listener, err := Listen(context.Background(), addr)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
		} else {
			conn.Close()
		}
	}()

	conn, err := dialer.DialContext(context.Background(), addr)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	defer conn.Close()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}
