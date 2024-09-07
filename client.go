package revdial

import (
	"fmt"
	"net"
)

type client struct {
	conn net.Conn
}

func newClient(conn net.Conn) *client {
	return &client{
		conn: conn,
	}
}

func (c *client) register() error {
	return nil
}

func (c *client) init(methods []byte) error {
	buf := make([]byte, 0, 2+len(methods))
	buf = append(buf, byte(v1))
	buf = append(buf, byte(len(methods)))
	buf = append(buf, methods...)

	_, err := c.conn.Write(buf)

	if err != nil {
		return fmt.Errorf("failed to write auth methods: %w", err)
	}

	resp := make([]byte, 2)

	_, err = c.conn.Read(resp)
	if err != nil {
		return fmt.Errorf("failed to read auth method response: %w", err)
	}

	if resp[0] != byte(v1) {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}

	c.handleAuth(authMethod(resp[1]))

	return err
}

func (c *client) Close() error {
	return c.conn.Close()
}

func (c *client) handleAuth(method authMethod) error {
	switch method {
	case noAuth:
		return nil
	case noAcceptableAuthMethod:
		return fmt.Errorf("no acceptable auth method")
	default:
		return fmt.Errorf("unsupported auth method: %d", method)
	}
}

func (c *client) handleRegister() error {
	buf := make([]byte, 2)
	buf[0] = byte(v1)
	buf[1] = byte(register)

	_, err := c.conn.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write connect command: %w", err)
	}

	resp := make([]byte, 2)
	_, err = c.conn.Read(resp)
	if err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if version(resp[0]) != v1 {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}

	res := result(resp[1])
	switch res {
	case success:
		return nil
	case failure:
		return fmt.Errorf("failed to register")
	default:
		return fmt.Errorf("unexpected result: %d", res)
	}
}
