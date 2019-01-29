package user

import (
	"context"
)

type key int

const userKey key = iota

// User represents a userof the system.
type User struct {
	Username    string
	Groups      []string
	Mail        string
	DisplayName string
}

// ContextGetUser returns the user if set in the given context.
func ContextGetUser(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userKey).(*User)
	return u, ok
}

// ContextMustGetUser panics if user it not in context.
func ContextMustGetUser(ctx context.Context) *User {
	u, ok := ContextGetUser(ctx)
	if !ok {
		panic("user not found in context")
	}
	return u
}

// ContextSetUser stores the user in the context.
func ContextSetUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// Manager is the interface to implement to manipulate users.
type Manager interface {
	GetUser(ctx context.Context, username string) (*User, error)
	GetUserGroups(ctx context.Context, username string) ([]string, error)
	IsInGroup(ctx context.Context, username, group string) (bool, error)
}
