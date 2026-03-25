package registry

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/user/outgoing"
)

// NewFunc is the function that the outgoing user manager implementations
// should register at init time.
type NewFunc func(context.Context, map[string]any) (outgoing.Manager, error)

// NewFuncs is a map containing all the registered outgoing user managers.
var NewFuncs = map[string]NewFunc{}

// Register registers a new outgoing user manager new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
