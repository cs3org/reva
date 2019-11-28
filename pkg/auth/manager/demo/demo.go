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
	"context"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	credentials map[string]Credentials
}

// Credentials holds a pair of secret and userid
type Credentials struct {
	ID     *user.UserId
	Secret string
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	// m not used
	creds := getCredentials()
	return &manager{credentials: creds}, nil
}

func (m *manager) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.UserId, error) {
	if c, ok := m.credentials[clientID]; ok {
		if c.Secret == clientSecret {
			return c.ID, nil
		}
	}
	return nil, errtypes.InvalidCredentials(clientID)
}

func getCredentials() map[string]Credentials {
	return map[string]Credentials{
		"einstein": Credentials{
			Secret: "relativity",
			ID: &user.UserId{
				OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
				Idp:      "http://localhost:9998",
			},
		},
		"marie": Credentials{
			Secret: "radioactivity",
			ID: &user.UserId{
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Idp:      "http://localhost:9998",
			},
		},
		"richard": Credentials{
			Secret: "superfluidity",
			ID: &user.UserId{
				OpaqueId: "932b4540-8d16-481e-8ef4-588e4b6b151c",
				Idp:      "http://localhost:9998",
			},
		},
	}
}
