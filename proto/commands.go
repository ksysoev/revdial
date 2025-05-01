package proto

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type CommandType int

type Command interface {
	Type() CommandType
	ParsePayload(data any) error
}

const (
	ConnectCommandType CommandType = iota + 1
	CustomEventType
)

type ConnectCommand struct {
	ID uuid.UUID
}

func (c ConnectCommand) Type() CommandType {
	return ConnectCommandType
}

func (c ConnectCommand) ParsePayload(data any) error {
	if _, ok := data.(*uuid.UUID); ok {
		data = &c.ID
		return nil
	}

	return fmt.Errorf("failed to parse connect command: expected *uuid.UUID, got %T", data)
}

type CustomEventCommand struct {
	Name string          `json:"n"`
	Data json.RawMessage `json:"d"`
}

func NewCustomEventCommand(name string, data any) (*CustomEventCommand, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return &CustomEventCommand{}, fmt.Errorf("failed to marshal custom event data: %w", err)
	}
	return &CustomEventCommand{
		Name: name,
		Data: dataBytes,
	}, nil
}

func (c CustomEventCommand) Type() CommandType {
	return CustomEventType
}

func (c CustomEventCommand) ParsePayload(data any) error {
	if err := json.Unmarshal(c.Data, data); err != nil {
		return fmt.Errorf("failed to unmarshal custom event data: %w", err)
	}

	return nil
}
