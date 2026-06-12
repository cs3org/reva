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

// Package appauth provides a credential strategy for app password authentication.
// It reads HTTP Basic Auth credentials and emits them with type "appauth", so the
// gateway routes the Authenticate call to the appauth auth provider rather than
// the regular (e.g. LDAP) basic auth provider.
//
// Assign this strategy to specific user agents via credentials_by_user_agent in
// the HTTP auth middleware config. For example, routing all Nextcloud desktop
// client requests (User-Agent contains "mirall") to appauth while leaving other
// clients on the regular "basic" strategy avoids breaking LDAP auth.
package appauth

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/registry"
	"github.com/cs3org/reva/v3/pkg/auth"
)

func init() {
	registry.Register("appauth", New)
}

type strategy struct{}

// New returns a credential strategy that reads HTTP Basic Auth and emits
// credentials of type "appauth".
func New(_ map[string]any) (auth.CredentialStrategy, error) {
	return &strategy{}, nil
}

func (s *strategy) GetCredentials(_ http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	id, secret, ok := r.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("no basic auth provided")
	}
	return &auth.Credentials{Type: "appauth", ClientID: id, ClientSecret: secret}, nil
}

func (s *strategy) AddWWWAuthenticate(w http.ResponseWriter, r *http.Request, realm string) {
	if realm == "" {
		realm = r.Host
	}
	w.Header().Add("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
}
