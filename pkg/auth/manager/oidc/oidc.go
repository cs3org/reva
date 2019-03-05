package oidc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/user"
	oidc "github.com/coreos/go-oidc"
	"golang.org/x/oauth2"
)

var logger = log.New("oidc")

func init() {
	registry.Register("oidc", New)
}

type mgr struct{}

// New returns an auth manager implementation that validatet the oidc token to authenticate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	return &mgr{}, nil
}

func (am *mgr) Authenticate(ctx context.Context, clientID, token string) (*user.User, error) {

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // FIXME make configurable
		},
	}
	customHTTPClient := &http.Client{
		Transport: tr,
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)

	// Initialize a provider by specifying dex's issuer URL.
	// provider needs to be cached as when it is created
	// it will fetch the keys from the issuer using the .well-known
	// endpoint
	provider, err := oidc.NewProvider(ctx, "https://owncloud.localhost:8443")
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: "ownCloud"})

	idToken, err := verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("could not verify bearer token: %v", err)
	}
	//logger.Printf(ctx, "idToken %+v", idToken)
	// Extract custom claims.
	var claims struct {
		Email       string            `json:"email"`
		Verified    bool              `json:"email_verified"`
		Groups      []string          `json:"groups"`
		DisplayName string            `json:"display_name"`
		KCIdentity  map[string]string `json:"kc.identity"`
	}
	//var anyJSON map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %v", err)
	}
	//logger.Printf(ctx, "claims %+v", claims)
	if !claims.Verified {
		// FIXME
		//return nil, fmt.Errorf("email (%q) in returned claims was not verified", claims.Email)
	}
	//ctx.Value("claims") = claims
	return &user.User{
		Username:    claims.KCIdentity["kc.i.un"],
		Groups:      []string{},
		Mail:        claims.Email,
		DisplayName: claims.KCIdentity["kc.i.dn"],
	}, nil
}

type userNotFoundError string

func (e userNotFoundError) Error() string   { return string(e) }
func (e userNotFoundError) IsUserNotFound() {}
