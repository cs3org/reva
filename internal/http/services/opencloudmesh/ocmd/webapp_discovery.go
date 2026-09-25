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

package ocmd

import (
	"net/url"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// WebappTokenEndpoint resolves the sender token endpoint from discovery.
// It does not contact the network. A path-relative endpoint is resolved
// against the absolute discovery endPoint. The result may still be http;
// launch policy applies https separately.
func WebappTokenEndpoint(disco *wellknown.OcmDiscoveryData) (string, error) {
	if disco == nil {
		return "", errtypes.BadRequest("sender discovery is missing")
	}
	if !slices.Contains(disco.Capabilities, "exchange-token") {
		return "", errtypes.BadRequest("sender discovery has no exchange-token capability")
	}
	raw := disco.TokenEndPoint
	if strings.TrimSpace(raw) == "" {
		return "", errtypes.BadRequest("sender discovery has no tokenEndPoint")
	}
	if strings.TrimSpace(raw) != raw {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if parsed.User != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if pathRelativeReference(parsed) {
		base, baseErr := absoluteDiscoveryBase(disco.Endpoint)
		if baseErr != nil {
			return "", errtypes.BadRequest("malformed remote URL")
		}
		resolved := base.ResolveReference(parsed)
		if err := requireAbsoluteHTTPURL(resolved); err != nil {
			return "", errtypes.BadRequest("malformed remote URL")
		}
		return resolved.String(), nil
	}
	if err := requireAbsoluteHTTPURL(parsed); err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	return raw, nil
}

func pathRelativeReference(u *url.URL) bool {
	return u != nil && u.Scheme == "" && u.Host == "" && u.Opaque == "" && u.User == nil
}

func absoluteDiscoveryBase(endpoint string) (*url.URL, error) {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(endpoint) != endpoint {
		return nil, errtypes.BadRequest("malformed remote URL")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if err := requireAbsoluteHTTPURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}
