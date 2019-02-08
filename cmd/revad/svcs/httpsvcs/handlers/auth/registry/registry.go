package registry

import (
	"github.com/cernbox/reva/pkg/auth"
)

// NewFunc is the function that auth strategies
// should register at init time.
type NewFunc func(map[string]interface{}) (auth.Strategy, error)

// NewFuncs is a map containing all the registered auth strategies.
var NewFuncs = map[string]NewFunc{}

// Register registers a new auth strategy  new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
