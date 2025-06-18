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
	"context"
	"time"

	auth "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/golang-jwt/jwt"
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
}

type manager struct {
	conf *config
}

// claims are custom claims for the JWT token.
type claims struct {
	jwt.StandardClaims
	User  *user.User             `json:"user"`
	Scope map[string]*auth.Scope `json:"scope"`
}

func (c *config) ApplyDefaults() {
	if c.Expires == 0 {
		c.Expires = defaultExpiration
	}

	c.Secret = sharedconf.GetJWTSecret(c.Secret)
}

// New returns an implementation of the token manager that uses JWT as tokens.
func New(m map[string]interface{}) (token.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	if c.Secret == "" {
		return nil, errors.New("jwt: secret for signing payloads is not defined in config")
	}

	mgr := &manager{conf: &c}
	return mgr, nil
}

func (m *manager) MintToken(ctx context.Context, u *user.User, scope map[string]*auth.Scope) (string, error) {
	claims := claims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: getExpirationDate(m.conf.ExpiresNextWeekend, time.Duration(m.conf.Expires)*time.Second).Unix(),
			Issuer:    u.Id.Idp,
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		User:  u,
		Scope: scope,
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
	token, err := jwt.ParseWithClaims(tkn, &claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(m.conf.Secret), nil
	})

	if err != nil {
		return nil, nil, errors.Wrap(err, "error parsing token")
	}

	if claims, ok := token.Claims.(*claims); ok && token.Valid {
		return claims.User, claims.Scope, nil
	}

	return nil, nil, errtypes.InvalidCredentials("invalid token")
}
