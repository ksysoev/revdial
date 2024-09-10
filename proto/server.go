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

func NewServer(conn net.Conn) *Server {
	return &Server{
		state: atomic.Int32{},
		conn:  conn,
	}
}

func (s *Server) State() State {
	return State(s.state.Load())
}

func (s *Server) ID() uint16 {
	return s.id
}

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

func (s *Server) Close() error {
	s.state.Store(int32(StateDisconnected))

	return s.conn.Close()
}

func (s *Server) handleInit() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	if version(buf[0]) != v1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	nmethods := int8(buf[1])
	methods := make([]byte, nmethods)

	if _, err := s.conn.Read(methods); err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	for _, m := range methods {
		method := authMethod(m)
		err := s.handleAuth(method)

		if errors.Is(err, ErrUnsupportedAuthMethod) {
			continue
		}

		if err != nil {
			return fmt.Errorf("failed to handle auth method: %w", err)
		}

		return nil
	}

	if _, err := s.conn.Write([]byte{byte(v1), byte(noAcceptableAuthMethod)}); err != nil {
		return fmt.Errorf("failed to write auth method response: %w", err)
	}

	return fmt.Errorf("no acceptable auth method")
}

func (s *Server) handleAuth(method authMethod) error {
	switch method {
	case noAuth:
		if _, err := s.conn.Write([]byte{byte(v1), byte(noAuth)}); err != nil {
			return fmt.Errorf("failed to write auth method response: %w", err)
		}

		return nil
	default:
		return ErrUnsupportedAuthMethod
	}
}

func (s *Server) handleCommand() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	if version(buf[0]) != v1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	cmd := command(buf[1])
	switch cmd {
	case register:
		return s.handleRegister()
	case bind:
		return s.handleBind()
	default:
		return fmt.Errorf("unsupported command: %d", cmd)
	}
}

func (s *Server) handleRegister() error {
	if _, err := s.conn.Write([]byte{byte(v1), byte(success)}); err != nil {
		return fmt.Errorf("failed to write register response: %w", err)
	}

	if !s.state.CompareAndSwap(int32(StateProcessing), int32(StateRegistered)) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	return nil
}

func (s *Server) handleBind() error {
	buf := make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read bind request: %w", err)
	}

	s.id = binary.BigEndian.Uint16(buf)

	if _, err := s.conn.Write([]byte{byte(v1), byte(success)}); err != nil {
		return fmt.Errorf("failed to write bind response: %w", err)
	}

	if !s.state.CompareAndSwap(int32(StateProcessing), int32(StateBound)) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	return nil
}

func (s *Server) SendConnectCommand(id uint16) error {
	if s.state.Load() != int32(StateRegistered) {
		return fmt.Errorf("unexpected state: %d", s.state.Load())
	}

	buf := make([]byte, 4)
	buf[0] = byte(v1)
	buf[1] = byte(connect)
	binary.BigEndian.PutUint16(buf[2:], id)

	if _, err := s.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to send connect command: %w", err)
	}

	buf = make([]byte, 2)

	if _, err := s.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if version(buf[0]) != v1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	if result(buf[1]) != success {
		return fmt.Errorf("failed to connect: %d", buf[1])
	}

	return nil
}
