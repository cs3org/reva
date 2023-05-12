package node

import (
	"context"
	"io"
)

// readerKey is the key for a reader to use when reading metadata
type readerKey struct{}

// ContextWithReader makes a new context that contains a reader to use when reading metadata. Use it if you want to read metadata from a locked node.
func ContextWithReader(parent context.Context, r io.Reader) context.Context {
	return context.WithValue(parent, readerKey{}, r)
}

// ReaderFromContext returns the reader stored in a context, or nil if there isn't one.
func ReaderFromContext(ctx context.Context) io.Reader {
	s, _ := ctx.Value(readerKey{}).(io.Reader)
	return s
}
