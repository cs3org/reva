package auth

import (
	"context"
	"net/http"
)

// Manager is the interface to implement to authenticate users
type Manager interface {
	Authenticate(ctx context.Context, clientID, clientSecret string) error
}

// Credentials contains the client id and secret.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

// Strategy obtains Credentials from the request.
type Strategy interface {
	GetCredentials(r *http.Request) (*Credentials, error)
}
