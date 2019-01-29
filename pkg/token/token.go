package token

import (
	"context"
)

// Claims is the map of attributes to encode into a token
type Claims map[string]interface{}

// Manager is the interface to implement to sign and verify tokens
type Manager interface {
	ForgeToken(ctx context.Context, claims Claims) (string, error)
	DismantleToken(ctx context.Context, token string) (Claims, error)
}

/*
	ForgeUserToken(ctx context.Context, user *User) (string, error)
	DismantleUserToken(ctx context.Context, token string) (*User, error)

	ForgePublicLinkToken(ctx context.Context, pl *PublicLink) (string, error)
	DismantlePublicLinkToken(ctx context.Context, token string) (*PublicLink, error)
*/
