package proto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithUserPassAuth(t *testing.T) {
	serv := NewServer(nil)
	assert.Nil(t, serv.userPassAuth)
	assert.True(t, serv.noAuth)

	serv = NewServer(nil, WithUserPassAuth(func(username, password string) bool { return true }))
	assert.NotNil(t, serv.userPassAuth)
	assert.False(t, serv.noAuth)
}

func TestWithNoAuth(t *testing.T) {
	serv := NewServer(nil, WithNoAuth())
	assert.True(t, serv.noAuth)

	serv = NewServer(nil, WithUserPassAuth(func(username, password string) bool { return true }))
	assert.False(t, serv.noAuth)

	serv = NewServer(nil, WithNoAuth(), WithUserPassAuth(func(username, password string) bool { return true }))
	assert.True(t, serv.noAuth)
}
