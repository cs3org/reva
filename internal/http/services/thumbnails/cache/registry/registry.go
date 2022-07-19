package registry

import (
	"github.com/cs3org/reva/internal/http/services/thumbnails/cache"
)

// NewFunc is the function that thumbnails cache implementations
// should register at init time.
type NewFunc func(map[string]interface{}) (cache.Cache, error)

// NewFuncs is a map containing all the thumbnails cache backends.
var NewFuncs = map[string]NewFunc{}

// Register registers a new thumbnails cache function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
