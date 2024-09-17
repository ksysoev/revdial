package revdial

import (
	"context"
	"log"
)

//nolint:errcheck // ignore errors in example for readability purposes
func ExampleDialer() {
	ctx := context.Background()
	dialer := NewDialer(":0")

	if err := dialer.Start(ctx); err != nil {
		log.Fatalf("failed to start dialer: %v", err)
	}

	defer dialer.Stop()

	listener, err := Listen(context.Background(), dialer.Addr())
	if err != nil {
		log.Printf("failed to create listener: %v", err)
		return
	}

	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			return
		}
		// Here you can do something with the connection
		conn.Close()
	}()

	conn, err := dialer.DialContext(ctx)
	if err != nil {
		log.Printf("failed to dial: %v", err)
		return
	}

	conn.Close()

	// Output:
}
