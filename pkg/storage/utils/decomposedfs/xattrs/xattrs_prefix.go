//go:build !freebsd

package xattrs

// The default namespace for ocis. As non root users can only manipulate
// the user. namespace, which is what is used to store ownCloud specific
// metadata. To prevent name collisions with other apps, we are going to
// introduce a sub namespace "user.ocis."
const (
	OcisPrefix string = "user.ocis."
)
