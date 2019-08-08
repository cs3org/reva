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

	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	credentials map[string]string
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	// m not used
	creds := getCredentials()
	return &manager{credentials: creds}, nil
}

func (m *manager) Authenticate(ctx context.Context, clientID, clientSecret string) (context.Context, error) {
	if secret, ok := m.credentials[clientID]; ok {
		if secret == clientSecret {
			return ctx, nil
		}
	}
	return ctx, errtypes.InvalidCredentials(clientID)
}

func getCredentials() map[string]string {
	return map[string]string{
		"einstein": "relativity",
		"marie":    "radioactivity",
		"richard":  "superfluidity",
	}
}
