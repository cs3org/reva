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

// CodeFlowOCMShareClientID returns the providerId carried by the single
// code-flow OCM share in scopes. The id is share.Id.OpaqueId from a scope
// built by AddCodeFlowOCMShareScope. Legacy shares that still carry Token, and
// any non-OCM scope, are not candidates. No code-flow share returns ("", nil).
// A missing id, a malformed ocmshare payload, or more than one code-flow share
// returns an error so minting fails closed instead of choosing an entry.
func CodeFlowOCMShareClientID(scopes map[string]*authpb.Scope) (string, error) {
	for k, s := range scopes {
		if !strings.HasPrefix(k, "ocmshare:") {
			continue
		}
		if s == nil || s.Resource == nil {
			return "", errtypes.InternalError("resource should be json encoded")
		}
	}

	shares, err := sharesFromScopes(scopes)
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

func sharesFromScopes(scopes map[string]*authpb.Scope) ([]*ocmv1beta1.Share, error) {
	shares := []*ocmv1beta1.Share{}
	for k, s := range scopes {
		if !strings.HasPrefix(k, "ocmshare:") {
			continue
		}
		res := s.Resource
		if res.Decoder != "json" {
			return nil, errtypes.InternalError("resource should be json encoded")
		}
		var share ocmv1beta1.Share
		err := utils.UnmarshalJSONToProtoV1(res.Value, &share)
		if err != nil {
			return nil, err
		}
		shares = append(shares, &share)
	}
	return shares, nil
}
