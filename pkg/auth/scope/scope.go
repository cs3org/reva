// Copyright 2018-2021 CERN
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

package scope

import (
	"fmt"
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
)

// Verifier is the function signature which every scope verifier should implement.
type Verifier func(*authpb.Scope, interface{}) (bool, error)

var supportedScopes = map[string]Verifier{
	"user":         userScope,
	"publicshare":  publicshareScope,
	"resourceinfo": resourceinfoScope,
}

// VerifyScope is the function to be called when dismantling tokens to check if
// the token has access to a particular resource.
func VerifyScope(scopeMap map[string]*authpb.Scope, resource interface{}) (bool, error) {
	for k, scope := range scopeMap {
		for s, f := range supportedScopes {
			if strings.HasPrefix(k, s) {
				valid, err := f(scope, resource)
				if err != nil {
					continue
				}
				if valid {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func FormatScope(scopeType string, scope *authpb.Scope) (string, error) {
	// TODO(gmgigi96): check decoder type
	switch {
	case strings.HasPrefix(scopeType, "user"):
		// user scope
		var ref provider.Reference
		err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &ref)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s", ref.String(), scope.Role.String()), nil
	case strings.HasPrefix(scopeType, "publicshare"):
		// public share
		var pShare link.PublicShare
		err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &pShare)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("share:\"%s\" %s", pShare.Id.OpaqueId, scope.Role.String()), nil
	default:
		return "", errtypes.NotSupported("scope not yet supported")
	}
}
