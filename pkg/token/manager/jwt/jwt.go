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

package jwt

import (
	"context"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/dgrijalva/jwt-go"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("jwt", New)
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
	m := &manager{conf: c}
	return m, nil
}

type manager struct {
	conf *config
}

type config struct {
	Secret string `mapstructure:"secret"`
}

// TODO(labkode): resulting JSON contains internal protobuf fields:
//  "Username": "einstein",
//  "XXX_NoUnkeyedLiteral": {},
//  "XXX_sizecache": 0,
//  "XXX_unrecognized": null
//}
func (m *manager) MintToken(ctx context.Context, u *authv0alphapb.User) (string, error) {
	claims := &jwt.MapClaims{}
	if err := mapstructure.Decode(u, claims); err != nil {
		return "", errors.Wrap(err, "error decoding user into jwt claims")
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(m.conf.Secret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return tkn, nil
}

func (m *manager) DismantleToken(ctx context.Context, tkn string) (*authv0alphapb.User, error) {
	jt, err := jwt.Parse(tkn, func(token *jwt.Token) (interface{}, error) {
		return []byte(m.conf.Secret), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "error parsing token")
	}

	if !jt.Valid {
		return nil, errors.Wrap(err, "token invalid")

	}

	claims := jt.Claims.(jwt.MapClaims)
	u := &authv0alphapb.User{}
	if err := mapstructure.Decode(claims, u); err != nil {
		return nil, errors.Wrap(err, "error decoding claims into user")
	}
	return u, nil
}
