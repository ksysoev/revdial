package revdial

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

//nolint:errcheck // ignore errors in example for readability purposes
func ExampleDialer_withTLS() {
	// In a real application, you would load certificates from files
	// For this example, we're using variables to represent the certificates
	var (
		serverCert tls.Certificate // Server's certificate and private key
		clientCert tls.Certificate // Client's certificate and private key
		caCertPool *x509.CertPool  // Pool of trusted CA certificates
	)

	// Configure TLS for the Dialer (server)
	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12, // Require TLS 1.2 or higher
	}

	// Configure TLS for the Listener (client)
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   "example.com", // Must match the server certificate's DNS names
	}

	ctx := context.Background()

	// Create and start the Dialer with TLS
	dialer := NewDialer(":0", WithDialerTLSConfig(serverTLSConfig))
	if err := dialer.Start(ctx); err != nil {
		log.Fatalf("failed to start dialer: %v", err)
	}
	defer dialer.Stop()

	// Create the Listener with TLS
	listener, err := Listen(context.Background(), dialer.Addr(),
		WithListenerTLSConfig(clientTLSConfig))
	if err != nil {
		log.Printf("failed to create listener: %v", err)
		return
	}
	defer listener.Close()

	// Accept connections in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			return
		}
		// The connection is now encrypted with TLS
		// Here you can safely transmit sensitive data
		conn.Close()
	}()

	// Dial using the secure connection
	conn, err := dialer.DialContext(ctx)
	if err != nil {
		log.Printf("failed to dial: %v", err)
		return
	}
	conn.Close()

	// Output:
}

//nolint:errcheck // ignore errors in example for readability purposes
func ExampleDialer_withTLSAndAuth() {
	// TLS configuration (as in the previous example)
	var (
		serverCert tls.Certificate
		clientCert tls.Certificate
		caCertPool *x509.CertPool
	)

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   "example.com",
	}

	ctx := context.Background()

	// Create and start the Dialer with both TLS and authentication
	authFunc := func(username, password string) bool {
		// In a real application, validate credentials against a secure store
		return username == "admin" && password == "secret"
	}

	dialer := NewDialer(":0",
		WithDialerTLSConfig(serverTLSConfig),
		WithUserPassAuth(authFunc))

	if err := dialer.Start(ctx); err != nil {
		log.Fatalf("failed to start dialer: %v", err)
	}
	defer dialer.Stop()

	// Create the Listener with both TLS and authentication
	authOpt, err := WithUserPass("admin", "secret")
	if err != nil {
		log.Printf("failed to create auth option: %v", err)
		return
	}

	listener, err := Listen(context.Background(), dialer.Addr(),
		WithListenerTLSConfig(clientTLSConfig),
		authOpt)
	if err != nil {
		log.Printf("failed to create listener: %v", err)
		return
	}
	defer listener.Close()

	// Accept connections in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			return
		}
		// Connection is now both encrypted and authenticated
		conn.Close()
	}()

	// Dial using the secure and authenticated connection
	conn, err := dialer.DialContext(ctx)
	if err != nil {
		log.Printf("failed to dial: %v", err)
		return
	}
	conn.Close()

	// Output:
}
