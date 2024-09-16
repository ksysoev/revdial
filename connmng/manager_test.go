package connmng

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	manager := New()

	assert.NotNil(t, manager, "Expected new ConnManager instance, got nil")
	assert.Empty(t, manager.connections, "Expected connections to be empty")
	assert.Equal(t, 0, manager.current, "Expected current to be 0")
}
