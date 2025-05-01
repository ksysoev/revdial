package proto

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type CommandType string

// Command represents an interface that defines a command structure for client-server communication.
// It includes methods to retrieve the command type and parse input payload data.
// The Type method returns the command type, while ParsePayload parses the provided data into the command structure.
type Command interface {
	Type() CommandType
	ParsePayload(data any) error
}

const (
	ConnectCommandType CommandType = "connect"
)

type ConnectCommand struct {
	ID uuid.UUID
}

// Type returns the type of the ConnectCommand as a CommandType.
// It does not take any parameters.
// It returns a CommandType indicating the specific type of this command.
func (c ConnectCommand) Type() CommandType {
	return ConnectCommandType
}

// ParsePayload validates and assigns the provided data to the ConnectCommand's ID field.
// It takes a single parameter data of type any, expected to be a pointer to uuid.UUID.
// It returns an error if the data is not of type *uuid.UUID or cannot be parsed.
func (c ConnectCommand) ParsePayload(data any) error {
	if d, ok := data.(*uuid.UUID); ok {
		*d = c.ID
		return nil
	}

	return fmt.Errorf("failed to parse connect command: expected *uuid.UUID, got %T", data)
}

// CustomEventCommand represents a command containing a name and associated JSON raw data.
// The Name field specifies the command type, defined as CommandType.
// The Data field holds raw JSON data associated with the command.
type CustomEventCommand struct {
	Name CommandType     `json:"n"`
	Data json.RawMessage `json:"d"`
}

// NewCustomEventCommand creates a new CustomEventCommand with the given name and data.
// It takes name of type string and data of type any, representing the event's name and its associated payload.
// It returns a pointer to a CustomEventCommand and an error if the name is empty, uses reserved command names,
// or if data marshaling fails.
func NewCustomEventCommand(name string, data any) (*CustomEventCommand, error) {
	typeName, err := NewCommandType(name)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom event command: %w", err)
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal custom event data: %w", err)
	}

	return &CustomEventCommand{
		Name: typeName,
		Data: dataBytes,
	}, nil
}

// Type returns the CommandType of the CustomEventCommand.
// It takes no parameters.
// It returns a CommandType representing the type of the command.
func (c CustomEventCommand) Type() CommandType {
	return c.Name
}

// ParsePayload unmarshals the JSON raw data in the CustomEventCommand into the provided data structure.
// It takes a parameter data of type any, which must be a pointer to the desired structure to unmarshal into.
// It returns an error if the unmarshal operation fails or if the provided data is incompatible with the JSON structure.
func (c CustomEventCommand) ParsePayload(data any) error {
	if err := json.Unmarshal(c.Data, data); err != nil {
		return fmt.Errorf("failed to unmarshal custom event data: %w", err)
	}

	return nil
}

func NewCommandType(name string) (CommandType, error) {
	typeName := CommandType(name)
	switch typeName {
	case "":
		return "", fmt.Errorf("event name cannot be empty")
	case ConnectCommandType:
		return "", fmt.Errorf("event name cannot contain reserved command names")
	}

	return typeName, nil
}
