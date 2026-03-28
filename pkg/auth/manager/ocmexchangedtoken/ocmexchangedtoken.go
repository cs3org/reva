// Copyright 2018-2026 CERN
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

// Package ocmexchangedtoken validates exchanged JWTs for OCM DAV access.
// It is fully stateless: no gateway call, no repository write.
// The user and scopes are extracted from the JWT claims.
package ocmexchangedtoken

import (
	"context"
	"fmt"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/token"
	tokenregistry "github.com/cs3org/reva/v3/pkg/token/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ocmexchangedtoken", New)
}

type manager struct {
	tokenmgr token.Manager
}

type config struct {
	TokenManager  string                    `mapstructure:"token_manager"`
	TokenManagers map[string]map[string]any `mapstructure:"token_managers"`
}

func (c *config) ApplyDefaults() {
	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}
}

// New creates a new ocmexchangedtoken authentication manager.
func New(ctx context.Context, m map[string]any) (auth.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, errors.Wrap(err, "ocmexchangedtoken: error decoding config")
	}

	tokenmgr, err := getTokenManager(c.TokenManager, c.TokenManagers)
	if err != nil {
		return nil, err
	}

	return &manager{tokenmgr: tokenmgr}, nil
}

func (m *manager) Configure(ml map[string]any) error {
	return nil
}

// Authenticate validates an exchanged JWT and returns the embedded user and scopes.
// It does NOT call GetAcceptedUser -- the user is already in the JWT claims.
func (m *manager) Authenticate(ctx context.Context, shareID, bearerToken string) (*userpb.User, map[string]*authpb.Scope, error) {
	user, scopes, err := m.tokenmgr.DismantleToken(ctx, bearerToken)
	if err != nil {
		return nil, nil, errtypes.InvalidCredentials("invalid exchanged token")
	}

	ocmShares, err := scope.GetOCMSharesFromScopes(scopes)
	if err != nil || len(ocmShares) == 0 {
		return nil, nil, errtypes.InvalidCredentials("no OCM share scope in exchanged token")
	}

	if shareID != "" {
		found := false
		for _, s := range ocmShares {
			if s.Id.GetOpaqueId() == shareID {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, errtypes.InvalidCredentials("scope mismatch for share " + shareID)
		}
	} else {
		// Bare-root request (e.g. PROPFIND /dav/ocm/): allow only when the
		// token carries exactly one OCM share scope so downstream storage
		// resolution is unambiguous.
		if len(ocmShares) != 1 {
			return nil, nil, errtypes.InvalidCredentials("empty shareID requires exactly one OCM share scope")
		}
	}

	return user, scopes, nil
}

func getTokenManager(manager string, m map[string]map[string]any) (token.Manager, error) {
	if f, ok := tokenregistry.NewFuncs[manager]; ok {
		return f(m[manager])
	}
	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for token manager", manager))
}
