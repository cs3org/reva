package userctx

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

type key int

const (
	userKey key = iota
	idKey
)

// ContextGetUser returns the user if set in the given context.
func ContextGetUser(ctx context.Context) (*userpb.User, bool) {
	u, ok := ctx.Value(userKey).(*userpb.User)
	return u, ok
}

// ContextMustGetUser panics if user is not in context.
func ContextMustGetUser(ctx context.Context) *userpb.User {
	u, ok := ContextGetUser(ctx)
	if !ok {
		panic("user not found in context")
	}
	return u
}

// ContextSetUser stores the user in the context.
func ContextSetUser(ctx context.Context, u *userpb.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// ContextGetUserID returns the user if set in the given context.
func ContextGetUserID(ctx context.Context) (*userpb.UserId, bool) {
	u, ok := ctx.Value(idKey).(*userpb.UserId)
	return u, ok
}

// ContextSetUserID stores the userid in the context.
func ContextSetUserID(ctx context.Context, id *userpb.UserId) context.Context {
	return context.WithValue(ctx, idKey, id)
}
