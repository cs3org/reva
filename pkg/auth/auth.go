package auth

import (
	"context"
)

// Manager is the interface to implement to authenticate users
type Manager interface {
	Authenticate(ctx context.Context, clientID, clientSecret string) error
}
