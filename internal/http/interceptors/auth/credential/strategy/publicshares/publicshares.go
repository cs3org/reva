// Copyright 2018-2023 CERN
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

package publicshares

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/internal/http/interceptors/auth/credential/registry"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("publicshares", New)
}

const (
	headerShareToken        = "public-token"
	basicAuthPasswordPrefix = "password|"
)

type config struct {
	UseCookies bool `mapstructure:"use_cookies"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	var c config
	err := mapstructure.Decode(m, &c)
	return &c, err
}

type strategy struct {
	c *config
}

// New returns a new auth strategy that handles public share verification.
func New(m map[string]interface{}) (auth.CredentialStrategy, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing config")
	}
	return &strategy{c: c}, nil
}

func (s *strategy) GetCredentials(w http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	token := r.Header.Get(headerShareToken)
	if token == "" {
		token = r.URL.Query().Get(headerShareToken)
	}
	if token == "" {
		return nil, fmt.Errorf("no public token provided")
	}

	sharePassword := basicAuthPasswordPrefix
	if s.c.UseCookies {
		if password, err := r.Cookie("password"); err == nil {
			sharePassword += password.Value
			return &auth.Credentials{Type: "publicshares", ClientID: token, ClientSecret: sharePassword}, nil
		}
	}

	// We can ignore the username since it is always set to "public" in public shares.
	_, password, ok := r.BasicAuth()
	if ok {
		sharePassword += password
	}
	return &auth.Credentials{Type: "publicshares", ClientID: token, ClientSecret: sharePassword}, nil
}

func (s *strategy) AddWWWAuthenticate(w http.ResponseWriter, r *http.Request, realm string) {
	// TODO read realm from forwarded header?
}
