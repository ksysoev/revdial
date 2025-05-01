package revdial

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCert creates a test certificate and private key for testing TLS
func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "Failed to generate private key")

	// Generate self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		DNSNames:              []string{"localhost", "example.com"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err, "Failed to create certificate")

	// Create certificate pool
	certPool := x509.NewCertPool()
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err, "Failed to parse certificate")
	certPool.AddCert(cert)

	// Create tls.Certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
		Leaf:        cert,
	}

	return tlsCert, certPool
}

func TestListenerDialer(t *testing.T) {
	// Create a new dialer
	dialer := NewDialer(":0")

	if err := dialer.Start(t.Context()); err != nil {
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
	listener, err := Listen(t.Context(), addr)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	defer func() { _ = listener.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
		} else {
			_ = conn.Close()
		}
	}()

	conn, err := dialer.DialContext(t.Context())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	defer func() { _ = conn.Close() }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListenerDialer_WithUserPassAuth_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Create a new dialer
	dialer := NewDialer(":0", WithUserPassAuth(func(user, pass string) bool {
		return user == "user" && pass == "pass"
	}))

	if err := dialer.Start(ctx); err != nil {
		t.Fatalf("failed to start dialer: %v", err)
	}

	addr := dialer.listener.Addr().String()

	lop, err := WithUserPass("user", "pass")
	assert.NoError(t, err, "WithUserPass should not return an error")

	// Create a new listener
	listener, err := Listen(ctx, addr, lop)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	defer func() { _ = listener.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
			cancel()
		} else {
			_ = conn.Close()
		}
	}()

	conn, err := dialer.DialContext(ctx)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	defer func() { _ = conn.Close() }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListenerDialer_WithTLS_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Generate test certificates
	cert, certPool := generateTestCert(t)

	// Configure TLS for server (dialer)
	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}

	// Configure TLS for client (listener)
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   "example.com",
		MinVersion:   tls.VersionTLS12,
	}

	// Create a new dialer with TLS
	dialer := NewDialer(":0", WithDialerTLSConfig(serverTLSConfig))

	require.NoError(t, dialer.Start(ctx), "Failed to start dialer")

	defer func() { _ = dialer.Stop() }()

	addr := dialer.listener.Addr().String()

	// Create a new listener with TLS
	listener, err := Listen(ctx, addr, WithListenerTLSConfig(clientTLSConfig))
	require.NoError(t, err, "Failed to create listener")

	defer func() { _ = listener.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
			cancel()
		} else {
			_ = conn.Close()
		}
	}()

	conn, err := dialer.DialContext(ctx)
	require.NoError(t, err, "Failed to dial")

	defer func() { _ = conn.Close() }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListenerDialer_WithTLS_InvalidCert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Generate two different certificates
	cert1, certPool1 := generateTestCert(t)
	cert2, _ := generateTestCert(t)

	// Configure TLS for server (dialer) with cert1
	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert1},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool1,
		MinVersion:   tls.VersionTLS12,
	}

	// Configure TLS for client (listener) with cert2 (untrusted)
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert2},
		RootCAs:      certPool1,
		ServerName:   "example.com",
		MinVersion:   tls.VersionTLS12,
	}

	// Create a new dialer with TLS
	dialer := NewDialer(":0", WithDialerTLSConfig(serverTLSConfig))

	require.NoError(t, dialer.Start(ctx), "Failed to start dialer")

	defer func() { _ = dialer.Stop() }()

	addr := dialer.listener.Addr().String()

	// Create a new listener with TLS
	listener, err := Listen(ctx, addr, WithListenerTLSConfig(clientTLSConfig))
	// Should fail due to invalid certificate
	require.Error(t, err, "Expected error due to invalid certificate")

	if listener != nil {
		_ = listener.Close()
	}
}

func TestListenerDialer_WithTLSAndAuth_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Generate test certificates
	cert, certPool := generateTestCert(t)

	// Configure TLS for server (dialer)
	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}

	// Configure TLS for client (listener)
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   "example.com",
		MinVersion:   tls.VersionTLS12,
	}

	// Create a new dialer with TLS and auth
	dialer := NewDialer(":0",
		WithDialerTLSConfig(serverTLSConfig),
		WithUserPassAuth(func(user, pass string) bool {
			return user == "user" && pass == "pass"
		}))

	require.NoError(t, dialer.Start(ctx), "Failed to start dialer")

	defer func() { _ = dialer.Stop() }()

	addr := dialer.listener.Addr().String()

	authOpt, err := WithUserPass("user", "pass")
	require.NoError(t, err, "Failed to create auth option")

	// Create a new listener with TLS and auth
	listener, err := Listen(ctx, addr,
		WithListenerTLSConfig(clientTLSConfig),
		authOpt)
	require.NoError(t, err, "Failed to create listener")

	defer func() { _ = listener.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
			cancel()
		} else {
			_ = conn.Close()
		}
	}()

	conn, err := dialer.DialContext(ctx)
	require.NoError(t, err, "Failed to dial")

	defer func() { _ = conn.Close() }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}

func TestListenerDialer_WithEventHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	// Create a new dialer
	dialer := NewDialer(":0")

	if err := dialer.Start(ctx); err != nil {
		t.Fatalf("failed to start dialer: %v", err)
	}

	defer func() {
		err := dialer.Stop()
		if err != nil {
			t.Errorf("failed to stop dialer: %v", err)
		}
	}()

	addr := dialer.listener.Addr().String()

	expectedEventName := "testEvent"
	expectedEventData := "testData"

	recievedEvent := make(chan struct{})
	lop, err := WithEventHandler(expectedEventName, func(event Event) {
		var result string

		err := event.ParsePayload(&result)
		assert.NoError(t, err, "Failed to parse payload")
		assert.Equal(t, expectedEventData, result, "Expected event data to match")

		close(recievedEvent)
	})
	assert.NoError(t, err, "WithUserPass should not return an error")

	// Create a new listener
	listener, err := Listen(ctx, addr, lop)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	go func() {
		_, _ = listener.Accept()
	}()

	defer func() { _ = listener.Close() }()

	err = dialer.SendEvent(ctx, expectedEventName, expectedEventData)
	assert.NoError(t, err, "Failed to send event")

	select {
	case <-recievedEvent:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be received")
	}
}
