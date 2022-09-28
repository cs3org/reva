package ctx

import (
	"context"
)

// ContextGetResourcePath returns the resource path if set in the given context.
func ContextGetResourcePath(ctx context.Context) (string, bool) {
	p, ok := ctx.Value(pathKey).(string)
	return p, ok
}

// ContextGetResourcePath stores the resource path in the context.
func ContextSetResourcePath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, pathKey, path)
}
