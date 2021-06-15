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
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
)

func publicshareScope(scope *authpb.Scope, resource interface{}) (bool, error) {
	var share link.PublicShare
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return checkStorageRef(&share, v.GetRef()), nil
	case *provider.StatRequest:
		return checkStorageRef(&share, v.GetRef()), nil
	case *provider.ListContainerRequest:
		return checkStorageRef(&share, v.GetRef()), nil
	case *provider.InitiateFileDownloadRequest:
		return checkStorageRef(&share, v.GetRef()), nil

		// Editor role
		// TODO(ishank011): Add role checks,
		// need to return appropriate status codes in the ocs/ocdav layers.
	case *provider.CreateContainerRequest:
		return checkStorageRef(&share, v.GetRef()), nil
	case *provider.DeleteRequest:
		return checkStorageRef(&share, v.GetRef()), nil
	case *provider.MoveRequest:
		return checkStorageRef(&share, v.GetSource()) && checkStorageRef(&share, v.GetDestination()), nil
	case *provider.InitiateFileUploadRequest:
		return checkStorageRef(&share, v.GetRef()), nil

	case *link.GetPublicShareRequest:
		return checkPublicShareRef(&share, v.GetRef()), nil
	case string:
		return checkPath(v), nil
	}

	return false, errtypes.InternalError(fmt.Sprintf("resource type assertion failed: %+v", resource))
}

func checkStorageRef(s *link.PublicShare, r *provider.Reference) bool {
	// r: <id:<storage_id:$storageID node_id:$nodeID path:$path > >
	if r.ResourceId != nil && r.Path == "" { // path must be empty
		return s.ResourceId.StorageId == r.ResourceId.StorageId && s.ResourceId.OpaqueId == r.ResourceId.OpaqueId
	}

	// r: <path:"/public/$token" >
	if strings.HasPrefix(r.GetPath(), "/public/"+s.Token) {
		return true
	}
	return false
}

func checkPublicShareRef(s *link.PublicShare, ref *link.PublicShareReference) bool {
	// ref: <token:$token >
	return ref.GetToken() == s.Token
}

// GetPublicShareScope returns the scope to allow access to a public share and
// the shared resource.
func GetPublicShareScope(share *link.PublicShare, role authpb.Role) (map[string]*authpb.Scope, error) {
	val, err := utils.MarshalProtoV1ToJSON(share)
	if err != nil {
		return nil, err
	}
	return map[string]*authpb.Scope{
		"publicshare:" + share.Id.OpaqueId: &authpb.Scope{
			Resource: &types.OpaqueEntry{
				Decoder: "json",
				Value:   val,
			},
			Role: role,
		},
	}, nil
}
