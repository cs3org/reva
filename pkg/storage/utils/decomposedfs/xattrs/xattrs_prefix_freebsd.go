//go:build freebsd

package xattrs

// On FreeBSD the `user` namespace is implied through a separate syscall argument
// and will fail with invalid argument when you try to start an xattr name with user. or system.
// For that reason we drop the superfluous user. prefix for FreeBSD specifically.
const (
	OcisPrefix string = "ocis."
)
