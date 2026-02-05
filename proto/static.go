package proto

const (
	versionV1 byte = 1
	versionV2 byte = 2
)

const (
	noAuth                 byte = 0
	userPassAuth           byte = 2
	noAcceptableAuthMethod byte = 255
)

const (
	cmdRegister    byte = 1
	cmdConnect     byte = 2
	cmdBind        byte = 3
	cmdPing        byte = 4
	cmdCustomEvent byte = 5
	cmdMuxInit     byte = 10
	cmdMuxReady    byte = 11
)

const (
	resSuccess byte = 0
	resFailure byte = 1
)

// VersionV1 returns the V1 protocol version byte.
func VersionV1() byte {
	return versionV1
}

// VersionV2 returns the V2 protocol version byte.
func VersionV2() byte {
	return versionV2
}

// CmdBind returns the bind command byte.
func CmdBind() byte {
	return cmdBind
}

// ResSuccess returns the success response byte.
func ResSuccess() byte {
	return resSuccess
}
