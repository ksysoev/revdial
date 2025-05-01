package proto

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestConnectCommand_Type(t *testing.T) {
	command := ConnectCommand{ID: uuid.New()}
	expectedType := ConnectCommandType

	result := command.Type()

	assert.Equal(t, expectedType, result)
}

func TestConnectCommand_ParsePayload(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		shouldErr bool
	}{
		{name: "Valid UUID Pointer", input: &uuid.UUID{}, shouldErr: false},
		{name: "Invalid Data Type", input: "invalid", shouldErr: true},
		{name: "Nil Input", input: nil, shouldErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command := ConnectCommand{
				ID: uuid.New(),
			}
			err := command.ParsePayload(tc.input)

			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.input.(*uuid.UUID), &command.ID)
			}
		})
	}
}

func TestCustomEventCommand_Type(t *testing.T) {
	command := CustomEventCommand{Name: "test-event"}
	expectedType := CommandType("test-event")

	result := command.Type()

	assert.Equal(t, expectedType, result)
}

func TestCustomEventCommand_ParsePayload(t *testing.T) {
	tests := []struct {
		name      string
		data      json.RawMessage
		output    any
		shouldErr bool
	}{
		{"Valid JSON Data", json.RawMessage(`{"key":"value"}`), &map[string]string{"key": "value"}, false},
		{"Invalid JSON Data", json.RawMessage(`invalid`), &map[string]string{}, true},
		{"Nil Target Object", json.RawMessage(`{"key":"value"}`), nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command := CustomEventCommand{Data: tc.data}

			err := command.ParsePayload(tc.output)

			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.output, tc.output)
			}
		})
	}
}

func TestNewCustomEventCommand(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		payload   any
		shouldErr bool
	}{
		{"Valid Custom Event", "test-event", map[string]string{"key": "value"}, false},
		{"Empty Event Name", "", map[string]string{"key": "value"}, true},
		{"Reserved Event Name", string(ConnectCommandType), map[string]string{"key": "value"}, true},
		{"Invalid Payload", "test-event", func() {}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command, err := NewCustomEventCommand(tc.eventName, tc.payload)

			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, command)
				assert.Equal(t, CommandType(tc.eventName), command.Name)
				assert.JSONEq(t, string(command.Data), string(command.Data))
			}
		})
	}
}
