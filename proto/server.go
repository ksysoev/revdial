package proto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/google/uuid"
)

var ErrUnsupportedAuthMethod = fmt.Errorf("unsupported auth method")

type State int32

const (
	StateConnected    State = 0
	StateProcessing   State = 1
	StateRegistered   State = 2
	StateBound        State = 3
	StateDisconnected State = 4
)

type ServerOption func(*Server)

type Server struct {
	conn         net.Conn
	userPassAuth func(username, password string) bool
	state        State
	id           uuid.UUID
	noAuth       bool
	mu           sync.RWMutex
}

// NewServer creates a new Server instance with the given net.Conn.
// It initializes the Server's state to 0 and sets the connection to the provided conn.
func NewServer(conn net.Conn, opts ...ServerOption) *Server {
	s := &Server{
		conn: conn,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.userPassAuth == nil {
		s.noAuth = true
	}

	return s
}

// State returns the current state of the server.
func (s *Server) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.state
}

// ID returns the ID of binded connection.
func (s *Server) ID() uuid.UUID {
	return s.id
}

// Process processes the incoming requests on the server.
// It transitions the server state to StateProcessing if it is currently in StateConnected.
// It handles the initialization and commands received from the client.
// If any error occurs during the process, it closes the connection and returns an error.
// Returns nil if the process is successful.
func (s *Server) Process() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateConnected {
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	s.state = StateProcessing

	if err := s.handleInit(); err != nil {
		_ = s.conn.Close()
		return fmt.Errorf("failed to handle init: %w", err)
	}

	if err := s.handleCommand(); err != nil {
		_ = s.conn.Close()
		return fmt.Errorf("failed to handle command: %w", err)
	}

	return nil
}

// SendConnectCommand sends a connect command to the server with the specified ID.
// It checks if the server is in the registered state before sending the command.
// The function writes the command to the connection and reads the response.
// It returns an error if any of the operations fail, such as writing to the connection,
// reading the response, or encountering unexpected states or versions.
func (s *Server) SendConnectCommand(id uuid.UUID) error {
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

// SendPingCommand sends a ping command to the server to verify connectivity.
// It returns nil if the server responds successfully or an error if the operation fails.
// An error is returned if the server is not in the registered state, the request fails, or the response is unsuccessful.
func (s *Server) SendPingCommand() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRegistered {
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	req := make([]byte, 0, 17)
	req = append(req, cmdPing)

	resp, err := sendRequest(s.conn, req)
	if err != nil {
		return fmt.Errorf("failed to send ping command: %w", err)
	}

	if resp != resSuccess {
		return fmt.Errorf("failed to ping: %d", resp)
	}

	return nil
}

func (s *Server) EmitCustomEvent(eventName string, data any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRegistered {
		return fmt.Errorf("unexpected state: %d", s.state)
	}
	var event struct {
		name string
		data any
	}

	event.name = eventName
	event.data = data

	d, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal custom event: %w", err)
	}

	maxDataLen := uint64(^uint32(0))
	if uint64(len(d)) > maxDataLen {
		return fmt.Errorf("custom event data exceeds maximum length of %d bytes", maxDataLen)
	}

	dataLen := make([]byte, 4)
	binary.BigEndian.PutUint32(dataLen, uint32(len(d)))

	req := make([]byte, 0, 3+len(d))
	req = append(req, cmdCustomEvent)
	req = append(req, dataLen...)
	req = append(req, d...)

	resp, err := sendRequest(s.conn, req)
	if err != nil {
		return fmt.Errorf("failed to send custom event: %w", err)
	}

	if resp != resSuccess {
		return fmt.Errorf("failed to emit custom event: %d", resp)
	}

	return nil
}

// Close closes the server connection and updates the server state to "Disconnected".
// It returns an error if there was a problem closing the connection.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateDisconnected

	return s.conn.Close()
}

// handleInit handles the initialization process for the server.
// It reads the authentication methods from the connection and handles each method.
// If an unsupported authentication method is encountered, it continues to the next method.
// If an error occurs during the authentication process, it returns an error.
// If no acceptable authentication method is found, it writes a response to the connection and returns an error.
// The function returns an error if any read or write operation fails.
func (s *Server) handleInit() error {
	msg, err := readMsg(s.conn)
	if err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	nmethods := int8(msg)
	methods := make([]byte, nmethods)

	if _, err := s.conn.Read(methods); err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	for _, m := range methods {
		err := s.handleAuth(m)

		if errors.Is(err, ErrUnsupportedAuthMethod) {
			continue
		}

		if err != nil {
			return fmt.Errorf("failed to handle auth method: %w", err)
		}

		return nil
	}

	if _, err := s.conn.Write([]byte{versionV1, noAcceptableAuthMethod}); err != nil {
		return fmt.Errorf("failed to write auth method response: %w", err)
	}

	return fmt.Errorf("no acceptable auth method")
}

// handleAuth determines the appropriate authentication method for the connection and processes it accordingly.
// Returns an error if the method is unsupported or if any operation fails during processing.
func (s *Server) handleAuth(method byte) error {
	switch method {
	case noAuth:
		if !s.noAuth {
			return ErrUnsupportedAuthMethod
		}

		if _, err := s.conn.Write([]byte{versionV1, noAuth}); err != nil {
			return fmt.Errorf("failed to write auth method response: %w", err)
		}

		return nil
	case userPassAuth:
		if s.userPassAuth == nil {
			return ErrUnsupportedAuthMethod
		}

		return s.handleUserPassAuth()
	default:
		return ErrUnsupportedAuthMethod
	}
}

// handleUserPassAuth processes user password authentication for the connection.
// It reads and verifies the username and password, then responds with success or failure.
func (s *Server) handleUserPassAuth() error {
	userLen, err := sendRequest(s.conn, []byte{userPassAuth})
	if err != nil {
		return fmt.Errorf("failed to send user pass auth request: %w", err)
	}

	if int(userLen) == 0 {
		return fmt.Errorf("failed to read username length")
	}

	username := make([]byte, int(userLen))
	if _, err := s.conn.Read(username); err != nil {
		return fmt.Errorf("failed to read username: %w", err)
	}

	passLenBuf := make([]byte, 1)
	if _, err := s.conn.Read(passLenBuf); err != nil {
		return fmt.Errorf("failed to read username length: %w", err)
	}

	passLen := int(passLenBuf[0])
	if passLen == 0 {
		return fmt.Errorf("failed to read password length")
	}

	password := make([]byte, passLen)
	if _, err := s.conn.Read(password); err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if s.userPassAuth(string(username), string(password)) {
		if _, err := s.conn.Write([]byte{versionV1, resSuccess}); err != nil {
			return fmt.Errorf("failed to write auth response: %w", err)
		}
	} else {
		if _, err := s.conn.Write([]byte{versionV1, resFailure}); err != nil {
			return fmt.Errorf("failed to write auth response: %w", err)
		}

		return fmt.Errorf("authentication failed")
	}

	return nil
}

// handleCommand handles incoming commands from the client.
// It reads a command from the connection and performs the corresponding action based on the command type.
// Returns an error if there was a failure in reading the command or if the command is unsupported.
func (s *Server) handleCommand() error {
	msg, err := readMsg(s.conn)
	if err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	switch msg {
	case cmdRegister:
		return s.handleRegister()
	case cmdBind:
		return s.handleBind()
	default:
		return fmt.Errorf("unsupported command: %d", msg)
	}
}

// handleRegister handles the registration process for the server.
// It writes a register response to the connection and updates the server state to StateRegistered.
// Returns an error if writing the response fails or if the server state is not StateProcessing.
func (s *Server) handleRegister() error {
	buf := make([]byte, 16)
	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read register request: %w", err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %w", err)
	}

	s.id = id

	if _, err := s.conn.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write register response: %w", err)
	}

	if s.state != StateProcessing {
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	s.state = StateRegistered

	return nil
}

// handleBind handles the bind request from the client.
// It reads a 2-byte buffer from the connection and assigns the value to s.id.
// Then, it writes a bind response to the connection.
// If the state is not StateProcessing, it returns an error with the current state.
// Otherwise, it returns nil.
func (s *Server) handleBind() error {
	buf := make([]byte, 16)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read bind request: %w", err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %w", err)
	}

	s.id = id

	if _, err := s.conn.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write bind response: %w", err)
	}

	if s.state != StateProcessing {
		return fmt.Errorf("unexpected state: %d", s.state)
	}

	s.state = StateBound

	return nil
}

// WithUserPassAuth sets a custom username-password authentication function for the server.
func WithUserPassAuth(auth func(username, password string) bool) ServerOption {
	return func(s *Server) {
		s.userPassAuth = auth
	}
}

// WithNoAuth sets the server to operate without requiring authentication by configuring the noAuth option.
func WithNoAuth() ServerOption {
	return func(s *Server) {
		s.noAuth = true
	}
}
