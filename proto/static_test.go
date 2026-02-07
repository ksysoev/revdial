package proto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionV1(t *testing.T) {
	assert.Equal(t, byte(1), VersionV1())
}

func TestVersionV2(t *testing.T) {
	assert.Equal(t, byte(2), VersionV2())
}

func TestCmdBind(t *testing.T) {
	assert.Equal(t, byte(3), CmdBind())
}

func TestResSuccess(t *testing.T) {
	assert.Equal(t, byte(0), ResSuccess())
}
