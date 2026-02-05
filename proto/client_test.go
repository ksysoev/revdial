package proto

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	conn, _ := net.Pipe()

	defer func() { _ = conn.Close() }()

	client := NewClient(conn)

	if client.conn != conn {
		t.Errorf("Expected connection to be %v, but got %v", conn, client.conn)
	}

	if client.cmds == nil {
		t.Error("Expected cmds channel to be initialized, but got nil")
	}
}

func TestCommands(t *testing.T) {
	conn, _ := net.Pipe()

	defer func() { _ = conn.Close() }()

	client := NewClient(conn)

	commands := client.Commands()

	if commands == nil {
		t.Error("Expected non-nil commands channel, but got nil")
	}
}

func TestWithUserPass(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		password      string
		expectError   bool
		expectedPanic bool
	}{
		{
			name:        "valid username and password",
			username:    "user123",
			password:    "pass123",
			expectError: false,
		},
		{
			name:        "empty username and password",
			username:    "",
			password:    "",
			expectError: false,
		},
		{
			name:        "excessive username length",
			username:    string(make([]byte, 256)),
			password:    "pass123",
			expectError: true,
		},
		{
			name:        "excessive password length",
			username:    "user123",
			password:    string(make([]byte, 256)),
			expectError: true,
		},
		{
			name:          "panic on second auth setup",
			username:      "user123",
			password:      "pass123",
			expectedPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedPanic {
				assert.Panics(t, func() {
					option, err := WithUserPass(tt.username, tt.password)
					assert.NoError(t, err)

					client := &Client{
						authMode: userPassAuth, // Simulate an already set auth mode
					}

					option(client) // This should trigger a panic
				})

				return
			}

			option, err := WithUserPass(tt.username, tt.password)

			if tt.expectError {
				assert.Errorf(t, err, "expected error but got none")
				assert.Nil(t, option, "expected ClientOption to be nil")
			} else {
				assert.NoError(t, err, "unexpected error")
				assert.NotNil(t, option, "expected non-nil ClientOption")

				// Further check if the option modifies the client correctly
				client := &Client{}

				assert.NotPanics(t, func() {
					option(client)
				}, "unexpected panic on applying ClientOption")

				// Verify client state
				if len(tt.username) <= 255 && len(tt.password) <= 255 {
					assert.Equal(t, userPassAuth, client.authMode, "unexpected authMode")

					expectedTokenLength := 2 + len(tt.username) + len(tt.password)

					assert.Len(t, client.token, expectedTokenLength, "unexpected token length")
				}
			}
		})
	}
}
