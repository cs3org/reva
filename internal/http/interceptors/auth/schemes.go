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

package auth

import (
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
)

// credentialSchemes says what each credential strategy takes from a request,
// keyed by the name a credential_chain entry gives it.
//
// A strategy without an entry here is left out of the description rather than
// guessed at; TestEveryStrategyIsDescribed fails for one that is missing, so
// adding a strategy is also adding its description.
var credentialSchemes = map[string]auth.Scheme{
	"basic": {
		Name:        "basic",
		Kind:        auth.SchemeBasic,
		Description: "A username and password, as the sync clients send them.",
	},
	"appauth": {
		Name:        "appauth",
		Kind:        auth.SchemeBasic,
		Description: "A username and an app password, standing in for the account's own password.",
	},
	"bearer": {
		Name:        "bearer",
		Kind:        auth.SchemeBearer,
		Description: "An OAuth access token from the identity provider this deployment trusts. Also read from the access_token query parameter.",
	},
	"publicshares": {
		Name:        "publicshares",
		Kind:        auth.SchemeHeader,
		Param:       "public-token",
		Description: "The token of a public link, also read from the public-token query parameter. A link with a password wants that password as the HTTP basic password alongside it.",
	},
	"ocmshares": {
		Name:        "ocmshares",
		Kind:        auth.SchemeHeader,
		Param:       "ocm-token",
		Description: "The token of a share received from another site, also read from the ocm-token query parameter.",
	},
}

// tokenSchemes says how each token strategy expects an already issued reva
// token to come back, keyed by the name a token_strategy_chain entry gives it.
var tokenSchemes = map[string]auth.Scheme{
	"header": {
		Name:        "revaToken",
		Kind:        auth.SchemeHeader,
		Param:       appctx.TokenHeader,
		Description: "A reva token, as returned by authenticating. This is the header the services pass it in among themselves.",
	},
	"bearer": {
		Name:        "revaTokenBearer",
		Kind:        auth.SchemeBearer,
		Description: "A reva token, as returned by authenticating, in the Authorization header.",
	},
}

// Schemes describes how clients authenticate against a server whose auth
// middleware is configured by m.
//
// Both chains are reported: the credential chain is what a client presents to
// authenticate, the token strategy chain is how it carries the resulting
// token on later requests. Two schemes can land in the same place on the wire
// - an app password and a password are both HTTP basic - and are still
// reported apart, because they are different credentials to hold.
func Schemes(m map[string]any) ([]auth.Scheme, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	conf.applyDefaults()

	schemes := make([]auth.Scheme, 0, len(conf.CredentialChain)+len(conf.TokenStrategyChain))
	for _, name := range conf.CredentialChain {
		if s, ok := credentialSchemes[name]; ok {
			schemes = append(schemes, s)
		}
	}
	for _, name := range conf.TokenStrategyChain {
		if s, ok := tokenSchemes[name]; ok {
			schemes = append(schemes, s)
		}
	}
	return schemes, nil
}
