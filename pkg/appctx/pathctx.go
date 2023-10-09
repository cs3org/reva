package appctx

import "context"

// ResoucePathCtx is the key used in the opaque id for passing the resource path.
const ResoucePathCtx = "resource_path"

// ContextGetResourcePath returns the resource path if set in the given context.
func ContextGetResourcePath(ctx context.Context) (string, bool) {
	p, ok := ctx.Value(pathKey).(string)
	return p, ok
}

// ContextGetResourcePath stores the resource path in the context.
func ContextSetResourcePath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, pathKey, path)
}
