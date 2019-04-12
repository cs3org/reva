package token

import (
	"context"
)

type key int

const tokenKey key = iota

// Claims is the map of attributes to encode into a token
type Claims map[string]interface{}

// Manager is the interface to implement to sign and verify tokens
type Manager interface {
	MintToken(ctx context.Context, claims Claims) (string, error)
	DismantleToken(ctx context.Context, token string) (Claims, error)
}

// ContextGetToken returns the token if set in the given context.
func ContextGetToken(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(tokenKey).(string)
	return u, ok
}

// ContextMustGetToken panics if token is not in context.
func ContextMustGetToken(ctx context.Context) string {
	u, ok := ContextGetToken(ctx)
	if !ok {
		panic("token not found in context")
	}
	return u
}

// ContextSetToken stores the token in the context.
func ContextSetToken(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, tokenKey, t)
}
