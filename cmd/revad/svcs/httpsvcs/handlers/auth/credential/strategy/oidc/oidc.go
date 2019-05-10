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

package oidc

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

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/registry"

	"github.com/cs3org/reva/pkg/auth"
)

func init() {
	registry.Register("oidc", New)
}

type strategy struct{}

// New returns a new auth strategy that checks for oidc auth.
func New(m map[string]interface{}) (auth.CredentialStrategy, error) {
	return &strategy{}, nil
}

func (s *strategy) GetCredentials(w http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	// for time being just use OpenConnectID Connect
	hdr := r.Header.Get("Authorization")
	token := strings.TrimPrefix(hdr, "Bearer ")
	if token == "" {
		// TODO make realm configurable
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, r.Host))
		return nil, fmt.Errorf("no Bearer auth provided")
	}

	return &auth.Credentials{ClientSecret: token}, nil
}
