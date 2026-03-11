package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/ksysoev/revdial/mux"
)

// ServerV2 handles V2 protocol connections with multiplexing support.
// It wraps a Server for V1 compatibility while adding yamux session management.
type ServerV2 struct {
	*Server
	session   *yamux.Session
	muxConfig *mux.Config
	isV2      bool
}

// ServerV2Option is a configuration option for ServerV2.
type ServerV2Option func(*ServerV2)

// NewServerV2 creates a new ServerV2 instance with the given connection and options.
// It wraps the V1 server and adds V2 multiplexing capabilities.
func NewServerV2(conn net.Conn, baseOpts []ServerOption, v2Opts ...ServerV2Option) *ServerV2 {
	s2 := &ServerV2{
		Server:    NewServer(conn, baseOpts...),
		muxConfig: mux.DefaultConfig(),
		isV2:      false,
	}

	// Apply V2-specific options
	for _, opt := range v2Opts {
		opt(s2)
	}

	return s2
}

// Process processes the incoming connection and handles version negotiation.
// It reads the version byte and delegates to V1 or V2 processing accordingly.
// This provides backward compatibility with V1 clients.
func (s *ServerV2) Process() error {
	s.mu.Lock()

	if s.state != StateConnected {
		s.mu.Unlock()
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	s.state = StateProcessing
	s.mu.Unlock()

	// Handle auth initialization (same for V1 and V2)
	if err := s.handleInit(); err != nil {
		_ = s.conn.Close()
		return fmt.Errorf("failed to handle init: %w", err)
	}

	// Read the next command to determine if it's V1 or V2
	msg, err := readMsg(s.conn)
	if err != nil {
		_ = s.conn.Close()
		return fmt.Errorf("failed to read command: %w", err)
	}

	// Check if this is a V2 mux init command
	if msg == cmdMuxInit {
		return s.processV2AfterAuth()
	}

	// V1 protocol - handle register or bind
	s.mu.Lock()
	defer s.mu.Unlock()

	switch msg {
	case cmdRegister:
		return s.handleRegister()
	case cmdBind:
		return s.handleBind()
	default:
		return fmt.Errorf("unsupported command: %d", msg)
	}
}

// ProcessV2 handles V2-specific connection processing with multiplexing.
// It expects the connection to have already completed auth negotiation.
// It reads the mux init command, establishes the yamux session, and handles registration.
func (s *ServerV2) processV2AfterAuth() error {
	// Read mux configuration (12 bytes: 3 x uint32)
	// Note: This reads the client's config but we use our own server config for yamux
	configBuf := make([]byte, 12)
	if _, err := io.ReadFull(s.conn, configBuf); err != nil {
		return fmt.Errorf("failed to read mux config: %w", err)
	}

	// Send MuxReady response (using V1 format for compatibility)
	respBuf := make([]byte, 2)
	respBuf[0] = versionV1
	respBuf[1] = cmdMuxReady

	if _, err := s.conn.Write(respBuf); err != nil {
		return fmt.Errorf("failed to write mux ready: %w", err)
	}

	// Send our server configuration (12 bytes)
	configResp := make([]byte, 12)
	binary.BigEndian.PutUint32(configResp[0:4], s.muxConfig.MaxStreams)
	binary.BigEndian.PutUint32(configResp[4:8], s.muxConfig.StreamWindowSize)
	binary.BigEndian.PutUint32(configResp[8:12], s.muxConfig.ConnectionWindowSize)

	if _, err := s.conn.Write(configResp); err != nil {
		return fmt.Errorf("failed to write mux config: %w", err)
	}

	// Upgrade connection to yamux session (server mode)
	yamuxCfg := s.muxConfig.ToYamux()

	session, err := yamux.Server(s.conn, yamuxCfg)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	s.mu.Lock()
	s.session = session
	s.isV2 = true
	s.mu.Unlock()

	// Accept control stream (first stream from client)
	controlStream, err := session.AcceptStream()
	if err != nil {
		return fmt.Errorf("failed to accept control stream: %w", err)
	}

	// Read register command on control stream
	regMsg, err := readMsg(controlStream)
	if err != nil {
		return fmt.Errorf("failed to read register command: %w", err)
	}

	if regMsg != cmdRegister {
		return fmt.Errorf("expected register command, got: %d", regMsg)
	}

	// Read UUID
	buf := make([]byte, 16)
	if _, err := controlStream.Read(buf); err != nil {
		return fmt.Errorf("failed to read register UUID: %w", err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %w", err)
	}

	s.mu.Lock()
	s.id = id
	s.state = StateRegistered
	s.mu.Unlock()

	// Send success response (using V1 format for compatibility)
	if _, err := controlStream.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write register response: %w", err)
	}

	// Replace conn with control stream for command handling
	s.conn = controlStream

	return nil
}

// IsV2 returns true if this connection is using V2 protocol with multiplexing.
func (s *ServerV2) IsV2() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isV2
}

// Session returns the yamux session if this is a V2 connection.
// It returns nil for V1 connections.
func (s *ServerV2) Session() *yamux.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.session
}

// AcceptStream accepts a new stream from the yamux session.
// It returns an error if this is not a V2 connection.
func (s *ServerV2) AcceptStream() (net.Conn, error) {
	s.mu.RLock()
	isV2 := s.isV2
	session := s.session
	s.mu.RUnlock()

	if !isV2 || session == nil {
		return nil, fmt.Errorf("not a V2 connection")
	}

	stream, err := session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	return stream, nil
}

// SendConnectCommand sends a connect command in V2 or V1 mode.
// For V2, it uses the control stream. For V1, it falls back to the base Server implementation.
func (s *ServerV2) SendConnectCommand(id uuid.UUID) error {
	s.mu.RLock()
	isV2 := s.isV2
	s.mu.RUnlock()

	if isV2 {
		return s.SendConnectCommandV2(id)
	}

	// V1 fallback
	return s.Server.SendConnectCommand(id)
}

// SendConnectCommandV2 sends a connect command over the control stream in V2 mode.
// It behaves the same as V1 but uses the control stream.
func (s *ServerV2) SendConnectCommandV2(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRegistered {
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	req := make([]byte, 0, 17)
	req = append(req, cmdConnect)
	req = append(req, id[:]...)

	resp, err := sendRequest(s.conn, req)
	if err != nil {
		return fmt.Errorf("failed to send connect command: %w", err)
	}

	if resp != resSuccess {
		return fmt.Errorf("failed to connect: %d", resp)
	}

	return nil
}

// SendCustomEvent sends a custom event in V2 or V1 mode.
// For V2, it uses the control stream. For V1, it falls back to the base Server implementation.
func (s *ServerV2) SendCustomEvent(eventName string, data any) error {
	// Both V1 and V2 use the same logic since s.conn points to the control stream in V2
	return s.Server.SendCustomEvent(eventName, data)
}

// Close closes the ServerV2 connection. For V2 connections it closes the yamux
// session first (which unblocks any in-flight AcceptStream call) and then
// delegates to the base Server.Close to close the underlying TCP connection.
// For V1 connections it behaves identically to Server.Close.
func (s *ServerV2) Close() error {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session != nil {
		// Closing the session tears down yamux's internal goroutines and
		// causes AcceptStream to return immediately with an error.
		_ = session.Close()
	}

	return s.Server.Close()
}

// WithMuxConfig sets the multiplexing configuration for V2 connections.
func WithMuxConfig(config *mux.Config) ServerV2Option {
	return func(s *ServerV2) {
		s.muxConfig = config
	}
}
