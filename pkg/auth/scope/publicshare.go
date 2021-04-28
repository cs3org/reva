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
	"encoding/json"
	"fmt"
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

func publicshareScope(scope *authpb.Scope, resource interface{}) (bool, error) {
	var share *link.PublicShare
	err := json.Unmarshal(scope.Resource.Value, &share)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	case *provider.Reference:
		return checkStorageRef(share, v), nil
	case *link.PublicShareReference:
		return checkPublicShareRef(share, v), nil
	case string:
		return checkPath(share, v), nil
	}

	return false, errtypes.InternalError(fmt.Sprintf("resource type assertion failed: %+v", resource))
}

func checkStorageRef(s *link.PublicShare, r *provider.Reference) bool {
	// ref: <id:<storage_id:$storageID opaque_id:$opaqueID > >
	if r.GetId() != nil {
		return s.ResourceId.StorageId == r.GetId().StorageId && s.ResourceId.OpaqueId == r.GetId().OpaqueId
	}
	// ref: <path:"/public/$token" >
	if strings.HasPrefix(r.GetPath(), "/public/"+s.Token) {
		return true
	}
	return false
}

func checkPublicShareRef(s *link.PublicShare, ref *link.PublicShareReference) bool {
	// ref: <token:$token >
	return ref.GetToken() == s.Token
}

func checkPath(s *link.PublicShare, path string) bool {
	paths := []string{
		"/dataprovider",
		"/data",
	}
	for _, p := range paths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
