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
		input     any
		name      string
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

				input, ok := tc.input.(*uuid.UUID)
				assert.True(t, ok, "Expected input to be of type *uuid.UUID")
				assert.Equal(t, input, &command.ID)
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
		output    any
		name      string
		data      json.RawMessage
		shouldErr bool
	}{
		{name: "Valid JSON Data", data: json.RawMessage(`{"key":"value"}`), output: map[string]string{"key": "value"}, shouldErr: false},
		{name: "Invalid JSON Data", data: json.RawMessage(`invalid`), output: map[string]string{}, shouldErr: true},
		{name: "Nil Target Object", data: json.RawMessage(`["key", "value"]`), output: nil, shouldErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command := CustomEventCommand{Data: tc.data}

			var output map[string]string
			err := command.ParsePayload(&output)

			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, output, tc.output)
			}
		})
	}
}

func TestNewCustomEventCommand(t *testing.T) {
	tests := []struct {
		payload   any
		name      string
		eventName string
		shouldErr bool
	}{
		{name: "Valid Custom Event", eventName: "test-event", payload: map[string]string{"key": "value"}, shouldErr: false},
		{name: "Empty Event Name", eventName: "", payload: map[string]string{"key": "value"}, shouldErr: true},
		{name: "Reserved Event Name", eventName: string(ConnectCommandType), payload: map[string]string{"key": "value"}, shouldErr: true},
		{name: "Invalid Payload", eventName: "test-event", payload: func() {}, shouldErr: true},
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
