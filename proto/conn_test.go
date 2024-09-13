package proto

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestReadMsg(t *testing.T) {
	tests := []struct {
		wantErr error
		name    string
		input   []byte
		want    byte
	}{
		{
			name:    "Valid message",
			input:   []byte{versionV1, 42},
			want:    42,
			wantErr: nil,
		},
		{
			name:    "Invalid version",
			input:   []byte{versionV1 + 1, 42},
			want:    0,
			wantErr: errors.New("unexpected version: 2"),
		},
		{
			name:    "Read error",
			input:   []byte{},
			want:    0,
			wantErr: errors.New("failed to read response: EOF"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.input)
			got, err := readMsg(reader)

			if (err != nil) != (tt.wantErr != nil) {
				t.Errorf("readMsg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("readMsg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("readMsg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSendRequest(t *testing.T) {
	tests := []struct {
		writeError error
		readError  error
		wantErr    error
		name       string
		req        []byte
		readData   []byte
		want       byte
	}{
		{
			name:     "Successful request",
			req:      []byte{42},
			readData: []byte{versionV1, 99},
			want:     99,
			wantErr:  nil,
		},
		{
			name:       "Write error",
			req:        []byte{42},
			writeError: errors.New("write error"),
			want:       0,
			wantErr:    fmt.Errorf("failed to write request: write error"),
		},
		{
			name:      "Read error",
			req:       []byte{42},
			readError: io.EOF,
			want:      0,
			wantErr:   fmt.Errorf("failed to read response: EOF"),
		},
		{
			name:     "Invalid version",
			req:      []byte{42},
			readData: []byte{versionV1 + 1, 99},
			want:     0,
			wantErr:  fmt.Errorf("unexpected version: %d", versionV1+1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConn{
				writeError: tt.writeError,
				readError:  tt.readError,
				readData:   tt.readData,
			}

			got, err := sendRequest(conn, tt.req)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("sendRequest() error = nil, wantErr %v", tt.wantErr)
					return
				}

				if err.Error() != tt.wantErr.Error() {
					t.Errorf("sendRequest() error = %v, wantErr %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Errorf("sendRequest() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("sendRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// mockConn is a mock implementation of io.ReadWriter for testing purposes.
type mockConn struct {
	writeError error
	readError  error
	readData   []byte
	writeData  []byte
}

func (m *mockConn) Read(p []byte) (n int, err error) {
	if m.readError != nil {
		return 0, m.readError
	}

	copy(p, m.readData)

	return len(m.readData), nil
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	if m.writeError != nil {
		return 0, m.writeError
	}

	m.writeData = append(m.writeData, p...)

	return len(p), nil
}

func (m *mockConn) Close() error {
	return nil
}
