package revdial

import (
	"errors"
	"fmt"
	"net"
)

var errUnsupportedAuthMethod = fmt.Errorf("unsupported auth method")

type server struct {
	conn net.Conn
}

func newServer(conn net.Conn) *server {
	return &server{
		conn: conn,
	}
}

func (s *server) handleConnection() error {
	err := s.handleInit()
	if err != nil {
		s.conn.Close()
		return fmt.Errorf("failed to handle init: %w", err)
	}

	return nil
}

func (s *server) Close() error {
	return s.conn.Close()
}

func (s *server) handleInit() error {
	buf := make([]byte, 2)
	_, err := s.conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	ver := version(buf[0])

	if ver != v1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	nmethods := int8(buf[1])
	methods := make([]byte, nmethods)
	_, err = s.conn.Read(methods)
	if err != nil {
		return fmt.Errorf("failed to read auth methods: %w", err)
	}

	for _, m := range methods {
		method := authMethod(m)
		err := s.handleAuth(method)
		if errors.Is(err, errUnsupportedAuthMethod) {
			continue
		}

		if err != nil {
			return fmt.Errorf("failed to handle auth method: %w", err)
		}
	}

	_, err = s.conn.Write([]byte{byte(v1), byte(noAcceptableAuthMethod)})
	if err != nil {
		return fmt.Errorf("failed to write auth method response: %w", err)
	}

	return fmt.Errorf("no acceptable auth method")
}

func (s *server) handleAuth(method authMethod) error {
	switch method {
	case noAuth:
		_, err := s.conn.Write([]byte{byte(v1), byte(noAuth)})
		if err != nil {
			return fmt.Errorf("failed to write auth method response: %w", err)
		}
		return nil
	default:
		return errUnsupportedAuthMethod
	}
}
