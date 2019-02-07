package registry

import "github.com/cernbox/reva/pkg/storage"

// NewFunc is the function that storage implementations
// should register at init time.
type NewFunc func(map[string]interface{}) (storage.FS, error)

// NewFuncs is a map containing all the registered storage backends.
var NewFuncs = map[string]NewFunc{}

// Register registers a new storage backend new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
