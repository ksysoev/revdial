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

	"github.com/ksysoev/revdial/pool"
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

// TestListener_DetectsServerClose verifies that when the server (Dialer) stops,
// the client-side Listener detects the disconnect and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := NewDialer(":0")

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	listener, err := Listen(ctx, addr)
	require.NoError(t, err, "failed to create listener")

	defer func() { _ = listener.Close() }()

	// Accept must return with ErrListenerClosed once the server shuts down.
	// Start Accept before stopping the server so it is already blocking.
	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop closes the TCP listener which tears down all accepted control connections.
	// We call it in a goroutine because Stop() waits for internal goroutines that
	// are themselves unblocked only once the client-side detects the disconnect.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection")
	}
}

// TestListener_DetectsServerClose_V2 is the same scenario with V2 protocol enabled.
func TestListener_DetectsServerClose_V2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := NewDialer(":0")

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	listener, err := Listen(ctx, addr, WithEnableV2())
	require.NoError(t, err, "failed to create listener with V2")

	defer func() { _ = listener.Close() }()

	// Verify V2 negotiated successfully before we close the server.
	assert.True(t, listener.client.IsV2(), "expected V2 protocol")

	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after the
	// client detects the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close (V2)")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection (V2)")
	}
}

// TestListener_DetectsServerClose_WithTLS verifies that when the server (Dialer) stops
// on a TLS-secured connection, the V1 client-side Listener detects the disconnect
// and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose_WithTLS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert, certPool := generateTestCert(t)

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}

	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   "example.com",
		MinVersion:   tls.VersionTLS12,
	}

	dialer := NewDialer(":0", WithDialerTLSConfig(serverTLSConfig))

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	listener, err := Listen(ctx, addr, WithListenerTLSConfig(clientTLSConfig))
	require.NoError(t, err, "failed to create listener with TLS")

	defer func() { _ = listener.Close() }()

	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after the
	// client detects the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close (TLS)")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection (TLS)")
	}
}

// TestListener_DetectsServerClose_WithTLSAndV2 verifies that when the server (Dialer) stops
// on a TLS-secured V2 connection, the client-side Listener detects the disconnect
// and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose_WithTLSAndV2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cert, certPool := generateTestCert(t)

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS12,
	}

	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   "example.com",
		MinVersion:   tls.VersionTLS12,
	}

	dialer := NewDialer(":0", WithDialerTLSConfig(serverTLSConfig))

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	listener, err := Listen(ctx, addr, WithListenerTLSConfig(clientTLSConfig), WithEnableV2())
	require.NoError(t, err, "failed to create listener with TLS and V2")

	defer func() { _ = listener.Close() }()

	assert.True(t, listener.client.IsV2(), "expected V2 protocol")

	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after the
	// client detects the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close (TLS + V2)")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection (TLS + V2)")
	}
}

// TestListener_DetectsServerClose_WithAuth verifies that when the server (Dialer) stops
// on a password-authenticated V1 connection, the client-side Listener detects the disconnect
// and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose_WithAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := NewDialer(":0", WithUserPassAuth(func(user, pass string) bool {
		return user == "user" && pass == "pass"
	}))

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	authOpt, err := WithUserPass("user", "pass")
	require.NoError(t, err, "failed to create auth option")

	listener, err := Listen(ctx, addr, authOpt)
	require.NoError(t, err, "failed to create listener with auth")

	defer func() { _ = listener.Close() }()

	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after the
	// client detects the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close (auth)")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection (auth)")
	}
}

// TestListener_DetectsServerClose_MultipleListeners verifies that when the server (Dialer) stops,
// all connected client-side Listeners detect the disconnect and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose_MultipleListeners(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := NewDialer(":0")

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	const numListeners = 3

	listeners := make([]*Listener, numListeners)

	for i := range numListeners {
		l, err := Listen(ctx, addr)
		require.NoError(t, err, "failed to create listener %d", i)

		listeners[i] = l
	}

	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()

	acceptErrs := make([]chan error, numListeners)

	for i := range numListeners {
		ch := make(chan error, 1)
		acceptErrs[i] = ch
		l := listeners[i]

		go func() {
			_, err := l.Accept()
			ch <- err
		}()
	}

	// Give the goroutines a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after all
	// clients detect the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	for i, ch := range acceptErrs {
		select {
		case err := <-ch:
			assert.ErrorIs(t, err, ErrListenerClosed, "listener %d: expected ErrListenerClosed after server close", i)
		case <-ctx.Done():
			t.Errorf("listener %d: Accept did not return after server closed the connection", i)
		}
	}

	require.NoError(t, <-stopErr, "dialer.Stop() failed")
}

// TestListener_DetectsServerClose_WithPool verifies that when the server (Dialer) stops,
// a V2 client using a connection pool detects the disconnect and Accept returns ErrListenerClosed.
func TestListener_DetectsServerClose_WithPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := NewDialer(":0")

	require.NoError(t, dialer.Start(ctx), "failed to start dialer")

	addr := dialer.listener.Addr().String()

	listener, err := Listen(ctx, addr, WithEnableV2(), WithPoolConfig(pool.DefaultConfig()))
	require.NoError(t, err, "failed to create listener with V2 and pool")

	defer func() { _ = listener.Close() }()

	assert.True(t, listener.client.IsV2(), "expected V2 protocol")

	acceptErr := make(chan error, 1)

	go func() {
		_, err := listener.Accept()
		acceptErr <- err
	}()

	// Give the goroutine a moment to enter Accept, then stop the server.
	// A ready-channel before Accept() would still race (the goroutine can be
	// preempted between close(ready) and the blocking Accept call), so a short
	// sleep is the most honest approximation; the 5 s test timeout is the safety net.
	time.Sleep(50 * time.Millisecond)

	// Stop() blocks until internal goroutines drain, which only unblock after the
	// client detects the disconnect. Run it in a goroutine and capture the error.
	stopErr := make(chan error, 1)

	go func() { stopErr <- dialer.Stop() }()

	select {
	case err := <-acceptErr:
		assert.ErrorIs(t, err, ErrListenerClosed, "expected ErrListenerClosed after server close (V2 + pool)")
		require.NoError(t, <-stopErr, "dialer.Stop() failed")
	case <-ctx.Done():
		t.Error("Accept did not return after server closed the connection (V2 + pool)")
	}
}

func TestListenerDialer_V2Protocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a new dialer - it uses ServerV2 which supports both V1 and V2
	dialer := NewDialer(":0")

	require.NoError(t, dialer.Start(ctx), "Failed to start dialer")

	defer func() {
		err := dialer.Stop()
		require.NoError(t, err, "Failed to stop dialer")
	}()

	addr := dialer.listener.Addr().String()

	// Create a listener with V2 enabled
	listener, err := Listen(ctx, addr, WithEnableV2())
	require.NoError(t, err, "Failed to create listener with V2")

	defer func() { _ = listener.Close() }()

	// Verify V2 is being used
	assert.True(t, listener.client.IsV2(), "Expected V2 protocol to be used")
	assert.NotNil(t, listener.client.Session(), "Expected yamux session to be created")

	// Give time for registration to complete
	time.Sleep(100 * time.Millisecond)

	// Test that connections work with V2 (streams instead of TCP)
	done := make(chan struct{})

	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
		} else {
			// Write some data to verify stream works
			_, err := conn.Write([]byte("hello from V2"))
			if err != nil {
				t.Errorf("failed to write to connection: %v", err)
			}

			_ = conn.Close()
		}
	}()

	// Dial a connection
	conn, err := dialer.DialContext(ctx)
	require.NoError(t, err, "Failed to dial")

	defer func() { _ = conn.Close() }()

	// Read the data to verify stream works
	buf := make([]byte, 100)
	n, err := conn.Read(buf)
	require.NoError(t, err, "Failed to read from connection")
	assert.Equal(t, "hello from V2", string(buf[:n]), "Expected data to match")

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection to be accepted")
	}
}
