package thumbnails

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type ContextKey int

const (
	ContextKeyResource ContextKey = iota
)

func ContextSetResource(ctx context.Context, res *provider.ResourceInfo) context.Context {
	return context.WithValue(ctx, ContextKeyResource, res)
}

func ContextMustGetResource(ctx context.Context) *provider.ResourceInfo {
	v, ok := ctx.Value(ContextKeyResource).(*provider.ResourceInfo)
	if !ok {
		panic("resource not in context")
	}
	return v
}
