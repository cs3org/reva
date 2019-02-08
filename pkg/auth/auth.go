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

// CredentialStrategy obtains Credentials from the request.
type CredentialStrategy interface {
	GetCredentials(r *http.Request) (*Credentials, error)
}

// TokenStrategy obtains a token from the request.
// If token does not exist returns an empty string.
type TokenStrategy interface {
	GetToken(r *http.Request) string
}

// TokenWriter stores the token in a http response.
type TokenWriter interface {
	WriteToken(token string, w http.ResponseWriter)
}
