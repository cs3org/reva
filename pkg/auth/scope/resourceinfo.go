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
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
)

func resourceinfoScope(scope *authpb.Scope, resource interface{}) (bool, error) {
	var r provider.ResourceInfo
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &r)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return checkResourceInfo(&r, v.GetRef()), nil
	case *provider.StatRequest:
		return checkResourceInfo(&r, v.GetRef()), nil
	case *provider.ListContainerRequest:
		return checkResourceInfo(&r, v.GetRef()), nil
	case *provider.InitiateFileDownloadRequest:
		return checkResourceInfo(&r, v.GetRef()), nil

		// Editor role
		// TODO(ishank011): Add role checks,
		// need to return appropriate status codes in the ocs/ocdav layers.
	case *provider.CreateContainerRequest:
		return checkResourceInfo(&r, v.GetRef()), nil
	case *provider.DeleteRequest:
		return checkResourceInfo(&r, v.GetRef()), nil
	case *provider.MoveRequest:
		return checkResourceInfo(&r, v.GetSource()) && checkResourceInfo(&r, v.GetDestination()), nil
	case *provider.InitiateFileUploadRequest:
		return checkResourceInfo(&r, v.GetRef()), nil

	case string:
		return checkPath(v), nil
	}

	return false, errtypes.InternalError(fmt.Sprintf("resource type assertion failed: %+v", resource))
}

func checkResourceInfo(inf *provider.ResourceInfo, ref *provider.Reference) bool {
	// TODO @ishank011 con you explain how this is used?
	// ref: <resource_id:<storage_id:$storageID opaque_id:$opaqueID path:$path> >
	if ref.ResourceId != nil { // path can be empty or a relative path
		if inf.Id.StorageId == ref.ResourceId.StorageId && inf.Id.OpaqueId == ref.ResourceId.OpaqueId {
			if ref.Path == "" {
				// id only reference
				return true
			}
			// check path has same prefix below
		} else {
			return false
		}
	}
	// ref: <path:$path >
	if strings.HasPrefix(ref.GetPath(), inf.Path) {
		return true
	}
	return false
}

func checkPath(path string) bool {
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

// GetResourceInfoScope returns the scope to allow access to a resource info object.
func GetResourceInfoScope(r *provider.ResourceInfo, role authpb.Role) (map[string]*authpb.Scope, error) {
	val, err := utils.MarshalProtoV1ToJSON(r)
	if err != nil {
		return nil, err
	}
	return map[string]*authpb.Scope{
		"resourceinfo:" + r.Id.String(): &authpb.Scope{
			Resource: &types.OpaqueEntry{
				Decoder: "json",
				Value:   val,
			},
			Role: role,
		},
	}, nil
}
