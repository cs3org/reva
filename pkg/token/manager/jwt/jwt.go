// Copyright 2018-2024 CERN
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

package jwt

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	auth "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	authcache "github.com/cs3org/reva/v3/pkg/auth/cache"
	"github.com/cs3org/reva/v3/pkg/auth/scope/ocmshare"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

const defaultExpiration int64 = 86400 // 1 day

func init() {
	registry.Register("jwt", New)
}

type config struct {
	Secret             string `mapstructure:"secret"`
	Expires            int64  `mapstructure:"expires"`
	ExpiresNextWeekend bool   `mapstructure:"expires_next_weekend"`
	authcache.Config   `mapstructure:",squash"`
}

type manager struct {
	conf *config
	// The cache is per manager, never shared, because an entry is only valid
	// for the secret that verified it.
	cache *authcache.Cache
}

// claims are custom claims for the JWT token.
type claims struct {
	jwt.RegisteredClaims
	User  *user.User             `json:"user"`
	Scope map[string]*auth.Scope `json:"scope"`
	// OCMShareID is the JWT client_id claim (share opaque id), not the receiver FQDN.
	OCMShareID clientIDClaim `json:"client_id,omitzero"`
}

// clientIDClaim records whether client_id was present in the payload.
// encoding/json decodes an omitted claim, an empty string, and null into the
// same Go string, so presence has to be tracked on unmarshal.
type clientIDClaim struct {
	value   string
	present bool
	isNull  bool
}

func (c clientIDClaim) MarshalJSON() ([]byte, error) {
	if c.isNull {
		return []byte("null"), nil
	}
	return json.Marshal(c.value)
}

func (c *clientIDClaim) UnmarshalJSON(data []byte) error {
	c.present = true
	if bytes.Equal(data, []byte("null")) {
		c.isNull = true
		return nil
	}
	return json.Unmarshal(data, &c.value)
}

// IsZero reports an omitted claim so minting can skip client_id.
func (c clientIDClaim) IsZero() bool {
	return !c.present
}

func (c *config) ApplyDefaults() {
	if c.Expires == 0 {
		c.Expires = defaultExpiration
	}

	c.Secret = sharedconf.GetJWTSecret(c.Secret)
}

// New returns an implementation of the token manager that uses JWT as tokens.
func New(m map[string]any) (token.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	if c.Secret == "" {
		return nil, errors.New("jwt: secret for signing payloads is not defined in config")
	}

	mgr := &manager{conf: &c, cache: authcache.New(c.Config)}
	return mgr, nil
}

func (m *manager) MintToken(ctx context.Context, u *user.User, scope map[string]*auth.Scope) (string, error) {
	ocmShareID, err := ocmshare.CodeFlowOCMShareClientID(scope)
	if err != nil {
		return "", err
	}

	claims := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(getExpirationDate(m.conf.ExpiresNextWeekend, time.Duration(m.conf.Expires)*time.Second)),
			Issuer:    u.Id.Idp,
			Audience:  jwt.ClaimStrings{"reva"},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		User:       u,
		Scope:      scope,
		OCMShareID: clientIDClaim{value: ocmShareID, present: ocmShareID != ""},
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(m.conf.Secret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return tkn, nil
}

func getExpirationDate(expiresOnNextWeekend bool, expiration time.Duration) time.Time {
	if expiresOnNextWeekend {
		nextWeekend := getNextWeekend(time.Now())
		return setTime(nextWeekend, 23, 59, 59)
	}
	return time.Now().Add(expiration)
}

func getNextWeekend(now time.Time) time.Time {
	return now.Truncate(24 * time.Hour).Add(time.Duration(7-now.Weekday()) * 24 * time.Hour)
}

func setTime(t time.Time, hour, min, sec int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), hour, min, sec, 0, t.Location())
}

func (m *manager) DismantleToken(ctx context.Context, tkn string) (*user.User, map[string]*auth.Scope, error) {
	key := authcache.Key(tkn)
	if id, cached, ok := m.cache.Get(key); ok {
		if cached != nil {
			return nil, nil, cached
		}
		return id.User, id.Scopes, nil
	}

	token, err := jwt.ParseWithClaims(tkn, &claims{}, func(token *jwt.Token) (any, error) {
		return []byte(m.conf.Secret), nil
	})

	if err != nil {
		err = errors.Wrap(err, "error parsing token")
		m.cache.Set(key, nil, err, time.Time{})
		return nil, nil, err
	}

	c, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		err := errtypes.InvalidCredentials("invalid token")
		m.cache.Set(key, nil, err, time.Time{})
		return nil, nil, err
	}

	// A present client_id claim must be the code-flow share opaque id.
	// Tokens that omit the claim stay valid.
	if err := checkOCMClientIDClaim(c); err != nil {
		m.cache.Set(key, nil, err, time.Time{})
		return nil, nil, err
	}

	// Bound the entry by the expiration of the token itself, so a cache hit can
	// never resurrect a token that has run out.
	var notAfter time.Time
	if c.ExpiresAt != nil {
		notAfter = c.ExpiresAt.Time
	}
	m.cache.Set(key, &authcache.Identity{User: c.User, Scopes: c.Scope}, nil, notAfter)

	return c.User, c.Scope, nil
}

// checkOCMClientIDClaim compares a present client_id claim with the code-flow
// share opaque id. An omitted claim is allowed. An empty or null claim is not.
func checkOCMClientIDClaim(c *claims) error {
	if !c.OCMShareID.present {
		return nil
	}
	if c.OCMShareID.isNull || c.OCMShareID.value == "" {
		return errtypes.InvalidCredentials("ocm token client_id does not match share scope")
	}

	derived, err := ocmshare.CodeFlowOCMShareClientID(c.Scope)
	if err != nil {
		return err
	}
	if c.OCMShareID.value != derived {
		return errtypes.InvalidCredentials("ocm token client_id does not match share scope")
	}
	return nil
}

// ValidatedExpiresAt parses the token, validates it, and returns the expiration time.
func (m *manager) ValidatedExpiresAt(_ context.Context, tkn string) (time.Time, error) {
	token, err := jwt.ParseWithClaims(tkn, &claims{}, func(token *jwt.Token) (any, error) {
		// here we are called back by the JWT library to verify the signature of the token, which was encrypted
		// with the secret configured in Reva to mint tokens, therefore that's the one we have to return
		return []byte(m.conf.Secret), nil
	})
	if err != nil {
		return time.Time{}, errors.Wrap(err, "error parsing token")
	}
	if c, ok := token.Claims.(*claims); ok && token.Valid && c.ExpiresAt != nil {
		return c.ExpiresAt.Time, nil
	}
	return time.Time{}, errtypes.InternalError("token has no expiration")
}
