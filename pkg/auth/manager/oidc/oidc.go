// Copyright 2018-2019 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package oidc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
	oidc "github.com/coreos/go-oidc"
	"golang.org/x/oauth2"
)

func init() {
	registry.Register("oidc", New)
}

type mgr struct{}

// Claims will be stored in the context to be consumed by the oidc user manager
type Claims struct {
	Subject     string            `json:"sub"`
	Issuer      string            `json:"iss"`
	Email       string            `json:"email"`
	Verified    bool              `json:"email_verified"`
	Groups      []string          `json:"groups"`
	DisplayName string            `json:"display_name"`
	KCIdentity  map[string]string `json:"kc.identity"`
}

// ClaimsKey is the key for oidc claims in a context
var ClaimsKey struct{}

// New returns an auth manager implementation that validatet the oidc token to authenticate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	return &mgr{}, nil
}

func (am *mgr) Authenticate(ctx context.Context, clientID, token string) (context.Context, error) {

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // FIXME make configurable
		},
	}
	customHTTPClient := &http.Client{
		Transport: tr,
	}
	insecureCtx := context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)

	// Initialize a provider by specifying dex's issuer URL.
	// provider needs to be cached as when it is created
	// it will fetch the keys from the issuer using the .well-known
	// endpoint
	provider, err := oidc.NewProvider(insecureCtx, "https://owncloud.localhost:8443")
	if err != nil {
		return ctx, err
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: "ownCloud"})

	idToken, err := verifier.Verify(insecureCtx, token)
	if err != nil {
		return ctx, fmt.Errorf("could not verify bearer token: %v", err)
	}

	// Extract custom claims.
	var claims Claims

	if err := idToken.Claims(&claims); err != nil {
		return ctx, fmt.Errorf("failed to parse claims: %v", err)
	}

	// TODO(jfd): make it configurable.
	// if !claims.Verified {
	// return nil, fmt.Errorf("email (%q) in returned claims was not verified", claims.Email)
	// }

	// store claims in context
	ctx = context.WithValue(ctx, ClaimsKey, claims)

	return ctx, nil
}
