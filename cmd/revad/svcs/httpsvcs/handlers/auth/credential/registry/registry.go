package registry

import (
	"github.com/cernbox/reva/pkg/auth"
)

// NewCredentialFunc is the function that credential strategies
// should register at init time.
type NewCredentialFunc func(map[string]interface{}) (auth.CredentialStrategy, error)

// NewCredentialFuncs is a map containing all the registered auth strategies.
var NewCredentialFuncs = map[string]NewCredentialFunc{}

// Register registers a new auth strategy  new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewCredentialFunc) {
	NewCredentialFuncs[name] = f
}
