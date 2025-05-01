package proto

import (
	"encoding/json"

	"github.com/google/uuid"
)

type CommandType int

type Command interface {
	Type() CommandType
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

type CustomEventCommand struct {
	Name string          `json:"n"`
	Data json.RawMessage `json:"d"`
}

func (c CustomEventCommand) Type() CommandType {
	return CustomEventType
}
