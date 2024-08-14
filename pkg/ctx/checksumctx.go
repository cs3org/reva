package ctx

import "context"

// ContextGetChecksum returns the checksum if set in the given context.
func ContextGetChecksum(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(checksumKey).(string)
	return u, ok
}

// ContextWithChecksum returns the checksum if set in the given context. Otherwise it panics.
func ContextMustGetChecksum(ctx context.Context) string {
	u, ok := ctx.Value(checksumKey).(string)
	if !ok {
		panic("checksum not set in context")
	}
	return u
}

// ContextSetChecksum returns a new context with the given checksum.
func ContextSetChecksum(ctx context.Context, checksum string) context.Context {
	return context.WithValue(ctx, checksumKey, checksum)
}
