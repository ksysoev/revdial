package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
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

type Server struct {
	conn  net.Conn
	state atomic.Int32
	id    uint16
}

// NewServer creates a new Server instance with the given net.Conn.
// It initializes the Server's state to 0 and sets the connection to the provided conn.
func NewServer(conn net.Conn) *Server {
	return &Server{
		state: atomic.Int32{},
		conn:  conn,
	}
}

// State returns the current state of the server.
func (s *Server) State() State {
	return State(s.state.Load())
}

// ID returns the ID of binded connection.
func (s *Server) ID() uint16 {
	return s.id
}

// Process processes the incoming requests on the server.
// It transitions the server state to StateProcessing if it is currently in StateConnected.
// It handles the initialization and commands received from the client.
// If any error occurs during the process, it closes the connection and returns an error.
// Returns nil if the process is successful.
func (s *Server) Process() error {
	if !s.state.CompareAndSwap(int32(StateConnected), int32(StateProcessing)) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	if err := s.handleInit(); err != nil {
		s.conn.Close()
		return fmt.Errorf("failed to handle init: %w", err)
	}

	if err := s.handleCommand(); err != nil {
		s.conn.Close()
		return fmt.Errorf("failed to handle command: %w", err)
	}

	return nil
}

// SendConnectCommand sends a connect command to the server with the specified ID.
// It checks if the server is in the registered state before sending the command.
// The function writes the command to the connection and reads the response.
// It returns an error if any of the operations fail, such as writing to the connection,
// reading the response, or encountering unexpected states or versions.
func (s *Server) SendConnectCommand(id uint16) error {
	if s.state.Load() != int32(StateRegistered) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	buf := make([]byte, 4)
	buf[0] = versionV1
	buf[1] = cmdConnect
	binary.BigEndian.PutUint16(buf[2:], id)

	if _, err := s.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to send connect command: %w", err)
	}

	buf = make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if buf[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	if buf[1] != resSuccess {
		return fmt.Errorf("failed to connect: %d", buf[1])
	}

	return nil
}

// Close closes the server connection and updates the server state to "Disconnected".
// It returns an error if there was a problem closing the connection.
func (s *Server) Close() error {
	s.state.Store(int32(StateDisconnected))

	return s.conn.Close()
}

// handleInit handles the initialization process for the server.
// It reads the authentication methods from the connection and handles each method.
// If an unsupported authentication method is encountered, it continues to the next method.
// If an error occurs during the authentication process, it returns an error.
// If no acceptable authentication method is found, it writes a response to the connection and returns an error.
// The function returns an error if any read or write operation fails.
func (s *Server) handleInit() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	if buf[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	nmethods := int8(buf[1])
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

// handleAuth handles the authentication method for the server.
// It takes a byte parameter representing the authentication method.
// If the method is noAuth, it writes the authentication method response to the connection and returns nil.
// If the method is not supported, it returns an error of type ErrUnsupportedAuthMethod.
func (s *Server) handleAuth(method byte) error {
	switch method {
	case noAuth:
		if _, err := s.conn.Write([]byte{versionV1, noAuth}); err != nil {
			return fmt.Errorf("failed to write auth method response: %w", err)
		}

		return nil
	default:
		return ErrUnsupportedAuthMethod
	}
}

// handleCommand handles incoming commands from the client.
// It reads a command from the connection and performs the corresponding action based on the command type.
// Returns an error if there was a failure in reading the command or if the command is unsupported.
func (s *Server) handleCommand() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	if buf[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	switch buf[1] {
	case cmdRegister:
		return s.handleRegister()
	case cmdBind:
		return s.handleBind()
	default:
		return fmt.Errorf("unsupported command: %d", buf[1])
	}
}

// handleRegister handles the registration process for the server.
// It writes a register response to the connection and updates the server state to StateRegistered.
// Returns an error if writing the response fails or if the server state is not StateProcessing.
func (s *Server) handleRegister() error {
	if _, err := s.conn.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write register response: %w", err)
	}

	if !s.state.CompareAndSwap(int32(StateProcessing), int32(StateRegistered)) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	return nil
}

// handleBind handles the bind request from the client.
// It reads a 2-byte buffer from the connection and assigns the value to s.id.
// Then, it writes a bind response to the connection.
// If the state is not StateProcessing, it returns an error with the current state.
// Otherwise, it returns nil.
func (s *Server) handleBind() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read bind request: %w", err)
	}

	s.id = binary.BigEndian.Uint16(buf)

	if _, err := s.conn.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write bind response: %w", err)
	}

	if !s.state.CompareAndSwap(int32(StateProcessing), int32(StateBound)) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	return nil
}
