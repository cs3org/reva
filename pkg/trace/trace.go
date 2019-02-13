package trace

import "context"

type key int

const traceKey key = iota

// ContextGetTrace returns the Trace if set in the given context.
func ContextGetTrace(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(traceKey).(string)
	return u, ok
}

// ContextMustGetTrace panics if Trace it not in context.
func ContextMustGetTrace(ctx context.Context) string {
	t, ok := ContextGetTrace(ctx)
	if !ok {
		panic("trace not found in context")
	}
	return t
}

// ContextSetTrace stores the trace in the context.
func ContextSetTrace(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, traceKey, trace)
}
