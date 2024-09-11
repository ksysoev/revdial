package proto

const (
	versionV1 byte = 1
)

type authMethod byte

const (
	noAuth                 byte = 0
	noAcceptableAuthMethod byte = 255
)

const (
	cmdRegister byte = 1
	cmdConnect  byte = 2
	cmdBind     byte = 3
)

const (
	resSuccess byte = 0
	resFailure byte = 1
)
