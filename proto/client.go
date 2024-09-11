package proto

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
)

type ClientConnect struct {
	ID uint16
}

type Client struct {
	conn net.Conn
	cmds chan ClientConnect
	wg   sync.WaitGroup
}

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		cmds: make(chan ClientConnect),
	}
}

func (c *Client) Commands() <-chan ClientConnect {
	return c.cmds
}

func (c *Client) Register(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	if err := c.init([]byte{noAuth}); err != nil {
		cancel()
		return fmt.Errorf("failed to init client: %w", err)
	}

	if err := c.handleRegister(); err != nil {
		cancel()
		return fmt.Errorf("failed to handle register: %w", err)
	}

	c.wg.Add(2)

	go func() {
		defer c.wg.Done()
		<-ctx.Done()
		c.conn.Close()
	}()

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
	buf = append(buf, versionV1, byte(len(methods)))
	buf = append(buf, methods...)

	if _, err := c.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to write auth methods: %w", err)
	}

	resp := make([]byte, 2)

	if _, err := c.conn.Read(resp); err != nil {
		return fmt.Errorf("failed to read auth method response: %w", err)
	}

	if resp[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}

	return c.handleAuth(resp[1])
}

func (c *Client) Bind(id uint16) error {
	if err := c.init([]byte{noAuth}); err != nil {
		return fmt.Errorf("failed to init client: %w", err)
	}

	if err := c.handleBind(id); err != nil {
		return fmt.Errorf("failed to handle bind: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	defer c.wg.Wait()
	return c.conn.Close()
}

func (c *Client) handleAuth(method byte) error {
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
	buf[0] = versionV1
	buf[1] = cmdRegister

	if _, err := c.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to write connect command: %w", err)
	}

	resp := make([]byte, 2)

	if _, err := c.conn.Read(resp); err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if resp[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}

	switch resp[1] {
	case resSuccess:
		return nil
	case resFailure:
		return fmt.Errorf("failed to register")
	default:
		return fmt.Errorf("unexpected result: %d", resp[1])
	}
}

func (c *Client) handleBind(id uint16) error {
	buf := make([]byte, 4)
	buf[0] = versionV1
	buf[1] = cmdBind
	binary.BigEndian.PutUint16(buf[2:], id)

	if _, err := c.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to write connect command: %w", err)
	}

	resp := make([]byte, 2)

	if _, err := c.conn.Read(resp); err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if resp[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}

	if resp[1] != resSuccess {
		return fmt.Errorf("failed to bind")
	}

	return nil
}

func (c *Client) handleCommand(ctx context.Context) error {
	buf := make([]byte, 2)

	if _, err := c.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	if buf[0] != versionV1 {
		return fmt.Errorf("unexpected version: %d", buf[0])
	}

	switch buf[1] {
	case cmdConnect:
		return c.handleConnect(ctx)
	default:
		return fmt.Errorf("unsupported command: %d", buf[1])
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
	case c.cmds <- ClientConnect{
		ID: id,
	}:
	}

	if _, err := c.conn.Write([]byte{versionV1, resSuccess}); err != nil {
		return fmt.Errorf("failed to write connect response: %w", err)
	}

	return nil
}
