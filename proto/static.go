package proto

type version byte

const (
	v1 version = 1
)

type authMethod uint8

const (
	noAuth                 authMethod = 0
	noAcceptableAuthMethod authMethod = 255
)

type command byte

const (
	register command = 1
	connect  command = 2
	bind     command = 3
)

type result byte

const (
	success result = 0
	failure result = 1
)
