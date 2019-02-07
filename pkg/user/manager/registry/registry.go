package registry

import "github.com/cernbox/reva/pkg/user"

// NewFunc is the function that user managers
// should register at init time.
type NewFunc func(map[string]interface{}) (user.Manager, error)

// NewFuncs is a map containing all the registered user managers.
var NewFuncs = map[string]NewFunc{}

// Register registers a new user manager new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
