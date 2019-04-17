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
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/token/manager/registry"
	"github.com/dgrijalva/jwt-go"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("token-manager-jwt")

func init() {
	registry.Register("jwt", New)
}

type config struct {
	Secret string `mapstructure:"secret"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
		return nil, err
	}
	return c, nil
}
var errors = err.New("jwt")

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

func parseConfig(value map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(value, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *manager) MintToken(ctx context.Context, claims token.Claims) (string, error) {
	jc := jwt.MapClaims(claims)
	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), jc)

	tkn, err := t.SignedString([]byte(m.conf.Secret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", jc)
	}

	return tkn, nil
}

func (m *manager) DismantleToken(ctx context.Context, tkn string) (token.Claims, error) {
	jt, err := jwt.Parse(tkn, func(token *jwt.Token) (interface{}, error) {
		return []byte(m.conf.Secret), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "error parsing token")
	}

	if !jt.Valid {
		return nil, errors.Wrap(err, "token invalid")

	}

	jc := jt.Claims.(jwt.MapClaims)
	c := token.Claims(jc)

	return c, nil
}
