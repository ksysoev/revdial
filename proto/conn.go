package proto

import (
	"fmt"
	"io"
)

// sendRequest sends a request over the given connection and returns the response code and any error encountered.
// The request is sent as a byte slice, with the versionV1 byte followed by the request bytes.
// The function writes the request to the connection using conn.Write, and then calls readMsg to read the response code.
// If there is an error while writing the request, the function returns an error with a formatted error message.
func sendRequest(conn io.ReadWriter, req []byte) (byte, error) {
	buf := make([]byte, 0, 1+len(req))
	buf = append(buf, versionV1)
	buf = append(buf, req...)

	if _, err := conn.Write(buf); err != nil {
		return 0, fmt.Errorf("failed to write request: %w", err)
	}

	return readMsg(conn)
}

// readMsg reads a message from the given io.Reader and returns the message byte and an error, if any.
// It expects the message to be in a specific format where the first byte represents the version and the second byte represents the message content.
// If the read operation fails, it returns an error with a descriptive message.
// If the version byte is unexpected, it returns an error indicating the unexpected version.
func readMsg(conn io.Reader) (byte, error) {
	buf := make([]byte, 2)

	if _, err := conn.Read(buf); err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	if buf[0] != versionV1 {
		return 0, fmt.Errorf("unexpected version: %d", buf[0])
	}

	return buf[1], nil
}
