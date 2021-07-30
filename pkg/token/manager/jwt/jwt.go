// Copyright 2018-2021 CERN
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
	"crypto/sha256"
	"encoding/hex"
	"time"

	auth "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/golang-jwt/jwt"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/xujiajun/nutsdb"
)

const defaultExpiration int64 = 86400 // 1 day

func init() {
	registry.Register("jwt", New)
}

type config struct {
	Secret  string `mapstructure:"secret"`
	Expires int64  `mapstructure:"expires"`
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

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation of the token manager that uses JWT as tokens.
func New(value map[string]interface{}) (token.Manager, error) {
	c, err := parseConfig(value)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing config")
	}

	if c.Expires == 0 {
		c.Expires = defaultExpiration
	}

	c.Secret = sharedconf.GetJWTSecret(c.Secret)

	if c.Secret == "" {
		return nil, errors.New("jwt: secret for signing payloads is not defined in config")
	}

	return &manager{conf: c}, nil
}

func (m *manager) MintToken(ctx context.Context, u *user.User, scope map[string]*auth.Scope) (string, error) {
	ttl := time.Duration(m.conf.Expires) * time.Second
	claims := claims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
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

	return m.cacheAndReturnHash(tkn)
}

func (m *manager) DismantleToken(ctx context.Context, tkn string) (*user.User, map[string]*auth.Scope, error) {
	cachedToken, err := m.getCachedToken(tkn)
	if err != nil {
		return nil, nil, err
	}

	token, err := jwt.ParseWithClaims(cachedToken, &claims{}, func(token *jwt.Token) (interface{}, error) {
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

func (m *manager) getDBHandler() (*nutsdb.DB, error) {
	opt := nutsdb.DefaultOptions
	opt.Dir = "/var/tmp/reva/jwt"
	return nutsdb.Open(opt)
}

func (m *manager) cacheAndReturnHash(token string) (string, error) {
	h := sha256.New()
	if _, err := h.Write([]byte(token)); err != nil {
		return "", err
	}
	hash := hex.EncodeToString(h.Sum(nil))

	db, err := m.getDBHandler()
	if err != nil {
		return "", err
	}
	defer db.Close()

	if err := db.Update(
		func(tx *nutsdb.Tx) error {
			return tx.Put("jwt-tokens", []byte(hash), []byte(token), uint32(m.conf.Expires))
		}); err != nil {
		return "", err
	}

	return hash, nil
}

func (m *manager) getCachedToken(hashedToken string) (string, error) {
	db, err := m.getDBHandler()
	if err != nil {
		return "", err
	}
	defer db.Close()

	var token string
	if err := db.View(
		func(tx *nutsdb.Tx) error {
			e, err := tx.Get("jwt-tokens", []byte(hashedToken))
			if err != nil {
				return err
			}
			token = string(e.Value)
			return nil
		}); err != nil {
		return "", err
	}
	return token, nil
}
