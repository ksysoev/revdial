package proto

import (
	"net"
	"testing"
)

func TestNewClient(t *testing.T) {
	conn, _ := net.Pipe()
	defer conn.Close()

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
	client := NewClient(conn)

	commands := client.Commands()

	if commands == nil {
		t.Error("Expected non-nil commands channel, but got nil")
	}
}
