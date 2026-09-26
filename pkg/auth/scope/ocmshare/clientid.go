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

// Package ocmshare reads code-flow OCM share scopes without importing the
// parent scope package or the JWT token manager.
package ocmshare

import (
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
)

// SharesFromScopes decodes every scope whose key uses the ocmshare: prefix.
// Unrelated keys are ignored, including nil values. A matching nil scope, nil
// resource, or non-json decoder fails closed. No matching scopes returns a
// nil slice so callers can tell "nothing matched" from a decoded list.
func SharesFromScopes(scopes map[string]*authpb.Scope) ([]*ocmv1beta1.Share, error) {
	var shares []*ocmv1beta1.Share
	for k, s := range scopes {
		if !strings.HasPrefix(k, "ocmshare:") {
			continue
		}
		if s == nil || s.Resource == nil || s.Resource.Decoder != "json" {
			return nil, errtypes.InternalError("resource should be json encoded")
		}
		var share ocmv1beta1.Share
		err := utils.UnmarshalJSONToProtoV1(s.Resource.Value, &share)
		if err != nil {
			return nil, err
		}
		shares = append(shares, &share)
	}
	return shares, nil
}

// CodeFlowOCMShareClientID returns the opaque id of the single code-flow OCM
// share in scopes. Every ocmshare entry is decoded before legacy token-bearing
// shares are ignored, so a malformed legacy payload still fails. The returned
// id keeps the share's exact opaque spelling. No code-flow share returns
// ("", nil). A blank id or more than one code-flow share returns an error.
func CodeFlowOCMShareClientID(scopes map[string]*authpb.Scope) (string, error) {
	shares, err := SharesFromScopes(scopes)
	if err != nil {
		return "", err
	}

	var clientID string
	found := false
	for _, share := range shares {
		if share.GetToken() != "" {
			continue
		}
		opaqueID := share.GetId().GetOpaqueId()
		if strings.TrimSpace(opaqueID) == "" {
			return "", errtypes.InvalidCredentials("code-flow ocm share is missing provider id")
		}
		if found {
			return "", errtypes.InvalidCredentials("ambiguous code-flow ocm share scope")
		}
		clientID = opaqueID
		found = true
	}
	return clientID, nil
}
