package proto

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
)

type ClientConnet struct {
	ID uint16
}

type Client struct {
	conn net.Conn
	cmds chan ClientConnet
	wg   sync.WaitGroup
}

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		cmds: make(chan ClientConnet),
	}
}

func (c *Client) Commands() <-chan ClientConnet {
	return c.cmds
}

func (c *Client) Register(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	err := c.init([]byte{byte(noAuth)})
	if err != nil {
		return fmt.Errorf("failed to init client: %w", err)
	}

	err = c.handleRegister()
	if err != nil {
		return fmt.Errorf("failed to handle register: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		<-ctx.Done()
		c.conn.Close()
	}()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		defer close(c.cmds)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := c.handleCommand(ctx)
				if err != nil {
					slog.Error("failed to handle command", slog.Any("error", err))
					return
				}
			}
		}
	}()
	return nil
}

func (c *Client) init(methods []byte) error {
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

func (c *Client) Bind(id uint16) error {
	err := c.init([]byte{byte(noAuth)})
	if err != nil {
		return fmt.Errorf("failed to init client: %w", err)
	}

	err = c.handleBind(id)
	if err != nil {
		return fmt.Errorf("failed to handle bind: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	defer c.wg.Wait()
	return c.conn.Close()
}

func (c *Client) handleAuth(method authMethod) error {
	switch method {
	case noAuth:
		return nil
	case noAcceptableAuthMethod:
		return fmt.Errorf("no acceptable auth method")
	default:
		return fmt.Errorf("unsupported auth method: %d", method)
	}
}

func (c *Client) handleRegister() error {
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

func (c *Client) handleBind(id uint16) error {
	buf := make([]byte, 4)
	buf[0] = byte(v1)
	buf[1] = byte(bind)
	binary.BigEndian.PutUint16(buf[2:], id)

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

	if res != success {
		return fmt.Errorf("failed to bind")
	}

	return nil
}

func (c *Client) handleCommand(ctx context.Context) error {
	buf := make([]byte, 2)
	_, err := c.conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	ver := version(buf[0])
	if ver != v1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	cmd := command(buf[1])
	switch cmd {
	case connect:
		return c.handleConnect(ctx)
	default:
		return fmt.Errorf("unsupported command: %d", cmd)
	}
}

func (c *Client) handleConnect(ctx context.Context) error {
	buf := make([]byte, 2)
	if _, err := c.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read connect request: %w", err)
	}

	id := binary.BigEndian.Uint16(buf)

	select {
	case <-ctx.Done():
		return nil
	case c.cmds <- ClientConnet{
		ID: id,
	}:
	}

	if _, err := c.conn.Write([]byte{byte(v1), byte(success)}); err != nil {
		return fmt.Errorf("failed to write connect response: %w", err)
	}

	return nil
}
