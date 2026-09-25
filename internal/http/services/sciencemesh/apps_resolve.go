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

package sciencemesh

import (
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// requireHTTPSAppURI accepts only an absolute https application URI.
// Relative application URIs are rejected. The stored string is preserved.
func requireHTTPSAppURI(raw string) (string, error) {
	validated, err := ocmd.ValidateAbsoluteWebappURI(raw)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	parsed, err := url.Parse(validated)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return validated, nil
}

// resolveTokenEndpoint checks sender capability and token syntax, then
// requires https for the launch hop.
func resolveTokenEndpoint(disco *wellknown.OcmDiscoveryData) (string, error) {
	tokenURL, err := ocmd.WebappTokenEndpoint(disco)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(tokenURL)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return tokenURL, nil
}

// pathRelativeURL is a reference with no scheme and no authority.
// Network-path references and scheme-bearing values are not path-relative.
func pathRelativeURL(u *url.URL) bool {
	return u != nil && u.Scheme == "" && u.Host == "" && u.User == nil && u.Opaque == ""
}

func validateLaunchURL(u *url.URL) error {
	if u == nil {
		return errtypes.BadRequest("malformed remote URL")
	}
	if u.User != nil {
		return errtypes.BadRequest("remote URL must not include userinfo")
	}
	if u.Scheme == "" || u.Host == "" || u.Hostname() == "" || !u.IsAbs() {
		return errtypes.BadRequest("remote URL must be absolute")
	}
	if u.Scheme != "https" {
		return errtypes.BadRequest("remote URL must use https")
	}
	malformedHost := u.Host == "https:" || u.Host == "http:" || strings.Contains(u.Host, "://")
	malformedPath := strings.HasPrefix(u.Path, "//http://") || strings.HasPrefix(u.Path, "//https://")
	if malformedHost || malformedPath {
		return errtypes.BadRequest("malformed remote URL")
	}
	return nil
}
