package registry

import "github.com/cernbox/reva/pkg/token"

// NewFunc is the function that token managers
// should register at init time.
type NewFunc func(map[string]interface{}) (token.Manager, error)

// NewFuncs is a map containing all the registered token managers.
var NewFuncs = map[string]NewFunc{}

// Register registers a new token manager new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
