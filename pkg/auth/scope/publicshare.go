// Copyright 2018-2022 CERN
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
	"context"
	"fmt"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

func publicshareScope(ctx context.Context, scope *authpb.Scope, resource interface{}, logger *zerolog.Logger) (bool, error) {
	var share link.PublicShare
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.StatRequest:
		return checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.ListContainerRequest:
		return checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.InitiateFileDownloadRequest:
		return checkStorageRef(ctx, &share, v.GetRef()), nil
	case *appprovider.OpenInAppRequest:
		return checkStorageRef(ctx, &share, &provider.Reference{ResourceId: v.ResourceInfo.Id}), nil
	case *gateway.OpenInAppRequest:
		return checkStorageRef(ctx, &share, v.GetRef()), nil

	// Editor role
	// need to return appropriate status codes in the ocs/ocdav layers.
	case *provider.CreateContainerRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.TouchFileRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.DeleteRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.MoveRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetSource()) && checkStorageRef(ctx, &share, v.GetDestination()), nil
	case *provider.InitiateFileUploadRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.SetArbitraryMetadataRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil
	case *provider.UnsetArbitraryMetadataRequest:
		return hasRoleEditor(*scope) && checkStorageRef(ctx, &share, v.GetRef()), nil

	// App provider requests
	case *appregistry.GetDefaultAppProviderForMimeTypeRequest:
		return true, nil

	case *userv1beta1.GetUserByClaimRequest:
		return true, nil

	case *link.GetPublicShareRequest:
		return checkPublicShareRef(&share, v.GetRef()), nil
	case string:
		return checkResourcePath(v), nil
	}

	msg := fmt.Sprintf("resource type assertion failed: %+v", resource)
	logger.Debug().Str("scope", "publicshareScope").Msg(msg)
	return false, errtypes.InternalError(msg)
}

func checkStorageRef(ctx context.Context, s *link.PublicShare, r *provider.Reference) bool {
	// r: <resource_id:<storage_id:$storageID opaque_id:$opaqueID> >
	// OR
	// r: <resource_id:<storage_id:$public-storage-mount-ID opaque_id:$token/$relative-path> >
	if r.ResourceId != nil && r.Path == "" { // path must be empty
		return utils.ResourceIDEqual(s.ResourceId, r.GetResourceId()) || strings.HasPrefix(r.ResourceId.OpaqueId, s.Token)
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

// AddPublicShareScope adds the scope to allow access to a public share and
// the shared resource.
func AddPublicShareScope(share *link.PublicShare, role authpb.Role, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	// Create a new "scope share" to only expose the required fields `ResourceId` and `Token` to the scope.
	scopeShare := &link.PublicShare{ResourceId: share.ResourceId, Token: share.Token}
	val, err := utils.MarshalProtoV1ToJSON(scopeShare)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes["publicshare:"+share.Id.OpaqueId] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: role,
	}
	return scopes, nil
}

// GetPublicSharesFromScopes returns all the public shares in the given scope.
func GetPublicSharesFromScopes(scopes map[string]*authpb.Scope) ([]*link.PublicShare, error) {
	var shares []*link.PublicShare
	for k, s := range scopes {
		if strings.HasPrefix(k, "publicshare:") {
			res := s.Resource
			if res.Decoder != "json" {
				return nil, errtypes.InternalError("resource should be json encoded")
			}
			var share link.PublicShare
			err := utils.UnmarshalJSONToProtoV1(res.Value, &share)
			if err != nil {
				return nil, err
			}
			shares = append(shares, &share)
		}
	}
	return shares, nil
}
