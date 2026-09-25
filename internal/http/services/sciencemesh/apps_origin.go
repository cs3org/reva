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

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// senderDiscoveryOrigin prefers an absolute WebDAV origin and falls back to an
// absolute webapp origin. Path-relative values are skipped. A scheme, host,
// userinfo, or unsupported scheme is an error and is not replaced.
func senderDiscoveryOrigin(protocols []*ocmpb.Protocol) (string, error) {
	if origin, err := firstAbsoluteOrigin(protocolURIs(protocols, "webdav")); err != nil || origin != "" {
		return origin, err
	}
	if origin, err := firstAbsoluteOrigin(protocolURIs(protocols, "webapp")); err != nil || origin != "" {
		return origin, err
	}
	return "", errtypes.NotFound("share has no absolute sender origin")
}

func protocolURIs(protocols []*ocmpb.Protocol, kind string) []string {
	uris := []string{}
	for _, p := range protocols {
		if p == nil {
			continue
		}
		switch kind {
		case "webdav":
			opts, ok := p.Term.(*ocmpb.Protocol_WebdavOptions)
			if !ok || opts == nil || opts.WebdavOptions == nil {
				continue
			}
			uris = append(uris, opts.WebdavOptions.Uri)
		case "webapp":
			opts, ok := p.Term.(*ocmpb.Protocol_WebappOptions)
			if !ok || opts == nil || opts.WebappOptions == nil {
				continue
			}
			uris = append(uris, opts.WebappOptions.Uri)
		}
	}
	return uris
}

func firstAbsoluteOrigin(raws []string) (string, error) {
	for _, raw := range raws {
		origin, ok, err := absoluteOrigin(raw)
		if err != nil {
			return "", err
		}
		if ok {
			return origin, nil
		}
	}
	return "", nil
}

func absoluteOrigin(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", false, errtypes.BadRequest("malformed remote URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, errtypes.BadRequest("malformed remote URL")
	}
	if pathRelativeURL(parsed) {
		return "", false, nil
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", false, err
	}
	return parsed.Scheme + "://" + parsed.Host, true, nil
}
