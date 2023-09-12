package registry

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
)

// NewFunc is the function that custom prometheus collectors implement
// should register at init time.
type NewFunc func(context.Context, map[string]interface{}) ([]prometheus.Collector, error)

// NewFuncs is a map containing all the registered collectors
var NewFuncs = map[string]NewFunc{}

// Register registers a new prometheus collector new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
