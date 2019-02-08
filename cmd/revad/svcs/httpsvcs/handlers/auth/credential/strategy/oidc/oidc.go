package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cernbox/reva/pkg/log"

	oidc "github.com/coreos/go-oidc"
)

var logger = log.New("oidc")

type user struct {
	email  string
	groups []string
}

// authorize verifies a bearer token and pulls user information form the claims.
func authorize(ctx context.Context, verifier *oidc.IDTokenVerifier, bearerToken string) (*user, error) {

	idToken, err := verifier.Verify(ctx, bearerToken)
	if err != nil {
		return nil, fmt.Errorf("could not verify bearer token: %v", err)
	}
	// Extract custom claims.
	var claims struct {
		Email    string   `json:"email"`
		Verified bool     `json:"email_verified"`
		Groups   []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %v", err)
	}
	if !claims.Verified {
		return nil, fmt.Errorf("email (%q) in returned claims was not verified", claims.Email)
	}
	return &user{claims.Email, claims.Groups}, nil
}

// AuthHandler is the auth middlware that authenticates requests using OpenIDConnect, LDAP,
// or other auth mechanism.
func AuthHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// for time being just use OpenConnectID Connect
		hdr := r.Header.Get("Authorization")
		token := strings.TrimPrefix(hdr, "Bearer ")

		// Initialize a provider by specifying dex's issuer URL.
		// provider needs to be cached as when it is created
		// it will fetch the keys from the issuer using the .well-known
		// endpoint
		provider, err := oidc.NewProvider(ctx, "http://0.0.0.0:5556/dex")
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		verifier := provider.Verifier(&oidc.Config{ClientID: "example-app"})
		user, err := authorize(ctx, verifier, token)
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		logger.Println(ctx, "user logged in: ", user)
		h.ServeHTTP(w, r)
	})
}
