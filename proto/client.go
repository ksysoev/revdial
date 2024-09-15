package proto

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

type ClientConnect struct {
	ID uuid.UUID
}

type Client struct {
	cancel context.CancelFunc
	conn   io.ReadWriteCloser
	cmds   chan ClientConnect
	wg     sync.WaitGroup
}

// NewClient creates a new instance of the Client struct.
// It takes a net.Conn as a parameter and returns a pointer to the Client struct.
// The Client struct represents a client connection and contains a connection and a channel for commands.
func NewClient(conn io.ReadWriteCloser) *Client {
	return &Client{
		conn: conn,
		cmds: make(chan ClientConnect),
	}
}

// Commands returns a channel that can be used to receive incoming commands from the server.
func (c *Client) Commands() <-chan ClientConnect {
	return c.cmds
}

// Register registers the client with the server.
// It takes a context.Context as a parameter and returns an error.
// The Register method establishes a connection with the server and handles the registration process.
func (c *Client) Register(ctx context.Context, id uuid.UUID) error {
	ctx, c.cancel = context.WithCancel(ctx)

	if err := c.establish([]byte{noAuth}); err != nil {
		c.cancel()
		return fmt.Errorf("failed to init client: %w", err)
	}

	if err := c.handleRegister(id); err != nil {
		c.cancel()
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
		defer c.cancel()
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

// Bind establishes a connection with the server and handles the binding process.
// It takes an ID as a parameter and returns an error if the binding process fails.
// The ID parameter represents the identifier used for binding.
// This function initializes the client and handles the bind operation.
// It returns an error if the client initialization or bind operation fails.
func (c *Client) Bind(id uuid.UUID) error {
	if err := c.establish([]byte{noAuth}); err != nil {
		return fmt.Errorf("failed to init client: %w", err)
	}

	if err := c.handleBind(id); err != nil {
		return fmt.Errorf("failed to handle bind: %w", err)
	}

	return nil
}

// Close closes the client connection.
// It cancels any pending requests and waits for all pending requests to complete before closing the connection.
// Returns an error if there was a problem closing the connection.
func (c *Client) Close() error {
	defer c.wg.Wait()
	c.cancel()

	return c.conn.Close()
}

// establish establishes a connection with the server by sending the authentication methods.
// It takes a byte slice of methods as input and returns an error if any.
// The function constructs a buffer with the version and length of methods, and appends the methods to it.
// It then writes the buffer to the connection.
// Next, it reads a response from the connection and checks if the version is as expected.
// Finally, it calls the handleAuth function with the response as input.
// If any error occurs during the process, it is returned with an appropriate error message.
func (c *Client) establish(methods []byte) error {
	req := make([]byte, 0, 1+len(methods))
	req = append(req, byte(len(methods)))
	req = append(req, methods...)

	resp, err := sendRequest(c.conn, req)
	if err != nil {
		return fmt.Errorf("failed to establish connection: %w", err)
	}

	return c.handleAuth(resp)
}

// handleAuth handles the authentication method for the client.
// It takes a byte parameter 'method' representing the authentication method.
// It returns an error if the authentication method is not supported or acceptable.
// If the authentication method is 'noAuth', it returns nil.
// If the authentication method is 'noAcceptableAuthMethod', it returns an error with the message "no acceptable auth method".
// If the authentication method is unsupported, it returns an error with the message "unsupported auth method: <method>".
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

// handleRegister sends a register command to the server and handles the response.
// It writes the command to the connection and reads the response from the connection.
// If the response indicates success, it returns nil. If the response indicates failure,
// it returns an error with the message "failed to register". If the response is unexpected,
// it returns an error with the message "unexpected result: <response>".
// If there is an error while writing the command or reading the response, it returns
// an error with the corresponding error message.
func (c *Client) handleRegister(id uuid.UUID) error {
	req := make([]byte, 0, 17)
	req = append(req, cmdRegister)
	req = append(req, id[:]...)

	resp, err := sendRequest(c.conn, req)
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	switch resp {
	case resSuccess:
		return nil
	case resFailure:
		return fmt.Errorf("failed to register")
	default:
		return fmt.Errorf("unexpected result: %d", resp)
	}
}

// handleBind sends a bind command to the server with the specified ID.
// It writes the command to the connection and reads the response.
// If the command is successful, it returns nil. Otherwise, it returns an error.
func (c *Client) handleBind(id uuid.UUID) error {
	req := make([]byte, 0, 17)
	req = append(req, cmdBind)
	req = append(req, id[:]...)

	resp, err := sendRequest(c.conn, req)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	if resp != resSuccess {
		return fmt.Errorf("failed to bind")
	}

	return nil
}

// handleCommand reads a command from the connection and handles it accordingly.
// It returns an error if there was a problem reading the command or if the command is unsupported.
// The function uses a buffer of size 2 to read the command bytes from the connection.
// The first byte is expected to be the version number, and the second byte is expected to be the command type.
// If the version number is not as expected, an error is returned.
// If the command type is cmdConnect, the function calls handleConnect to handle the connection request.
// If the command type is unsupported, an error is returned.
// The function takes a context.Context as a parameter to support cancellation and timeouts.
// It returns an error if there was a problem reading the command or if the command is unsupported.
func (c *Client) handleCommand(ctx context.Context) error {
	msg, err := readMsg(c.conn)
	if err != nil {
		return fmt.Errorf("failed to read command: %w", err)
	}

	switch msg {
	case cmdConnect:
		return c.handleConnect(ctx)
	default:
		return fmt.Errorf("unsupported command: %d", msg)
	}
}

// handleConnect handles the connection request from the client.
// It reads a 2-byte buffer from the connection and extracts the ID from it.
// Then, it sends a ClientConnect command with the extracted ID to the cmds channel.
// If the context is canceled before sending the command, it returns nil.
// After sending the command, it writes a connect response to the connection.
// Returns an error if there is any issue reading, writing, or sending the command.
func (c *Client) handleConnect(ctx context.Context) error {
	buf := make([]byte, 16)
	if _, err := c.conn.Read(buf); err != nil {
		return fmt.Errorf("failed to read connect request: %w", err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %w", err)
	}

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
