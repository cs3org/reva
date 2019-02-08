package registry

import "github.com/cernbox/reva/pkg/auth"

// NewTokenFunc is the function that token writers
// should register at init time.
type NewTokenFunc func(map[string]interface{}) (auth.TokenWriter, error)

// NewTokenFuncs is a map containing all the registered token writers.
var NewTokenFuncs = map[string]NewTokenFunc{}

// Register registers a new token writer strategy  new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewTokenFunc) {
	NewTokenFuncs[name] = f
}
