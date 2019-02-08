package registry

import "github.com/cernbox/reva/pkg/auth"

// NewTokenFunc is the function that token strategies
// should register at init time.
type NewTokenFunc func(map[string]interface{}) (auth.TokenStrategy, error)

// NewTokenFuncs is a map containing all the registered auth strategies.
var NewTokenFuncs = map[string]NewTokenFunc{}

// Register registers a new auth strategy  new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewTokenFunc) {
	NewTokenFuncs[name] = f
}
