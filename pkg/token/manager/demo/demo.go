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

package demo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"

	"github.com/cernbox/reva/pkg/token"
	"github.com/cernbox/reva/pkg/token/manager/registry"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("demo", New)
}

// New returns a new token manager.
func New(m map[string]interface{}) (token.Manager, error) {
	mngr := manager{}
	return &mngr, nil
}

type manager struct{}

func (m *manager) MintToken(ctx context.Context, claims token.Claims) (string, error) {
	token, err := encode(claims)
	if err != nil {
		return "", errors.Wrap(err, "error encoding claims")
	}
	return token, nil
}

func (m *manager) DismantleToken(ctx context.Context, token string) (token.Claims, error) {
	claims, err := decode(token)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding claims")
	}
	return claims, nil
}

// from https://stackoverflow.com/questions/28020070/golang-serialize-and-deserialize-back
// go binary encoder
func encode(m token.Claims) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(m)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// from https://stackoverflow.com/questions/28020070/golang-serialize-and-deserialize-back
// go binary decoder
func decode(str string) (token.Claims, error) {
	m := token.Claims{}
	by, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}
	b := bytes.Buffer{}
	b.Write(by)
	d := gob.NewDecoder(&b)
	err = d.Decode(&m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
