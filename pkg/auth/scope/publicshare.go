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
	"context"
	"fmt"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

func publicshareScope(ctx context.Context, scope *authpb.Scope, resource interface{}, logger *zerolog.Logger, client gatewayv1beta1.GatewayAPIClient, mgr token.Manager) (bool, error) {
	var share link.PublicShare
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *provider.StatRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *provider.ListContainerRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *provider.InitiateFileDownloadRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil

		// Editor role
		// TODO(ishank011): Add role checks,
		// need to return appropriate status codes in the ocs/ocdav layers.
	case *provider.CreateContainerRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *provider.DeleteRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *provider.MoveRequest:
		return checkStorageRef(ctx, &share, v.GetSource(), client, mgr) && checkStorageRef(ctx, &share, v.GetDestination(), client, mgr), nil
	case *provider.InitiateFileUploadRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil
	case *appregistry.GetAppProvidersRequest:
		return checkStorageRef(ctx, &share, &provider.Reference{ResourceId: v.ResourceInfo.Id}, client, mgr), nil
	case *appregistry.GetDefaultAppProviderForMimeTypeRequest:
		return true, nil
	case *appprovider.OpenInAppRequest:
		return checkStorageRef(ctx, &share, &provider.Reference{ResourceId: v.ResourceInfo.Id}, client, mgr), nil
	case *gatewayv1beta1.OpenInAppRequest:
		return checkStorageRef(ctx, &share, v.GetRef(), client, mgr), nil

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

func checkStorageRef(ctx context.Context, s *link.PublicShare, r *provider.Reference, client gatewayv1beta1.GatewayAPIClient, mgr token.Manager) bool {
	// r: <resource_id:<storage_id:$storageID opaque_id:$opaqueID> path:$path > >
	if r.ResourceId != nil && r.Path == "" { // path must be empty
		if utils.ResourceIDEqual(s.ResourceId, r.GetResourceId()) {
			return true
		}
		shareStat, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: s.ResourceId}})
		if err != nil || shareStat.Status.Code != rpcv1beta1.Code_CODE_OK {
			return false
		}

		userResp, err := client.GetUserByClaim(ctx, &userv1beta1.GetUserByClaimRequest{Claim: "userid", Value: shareStat.Info.Owner.OpaqueId})
		if err != nil || userResp.Status.Code != rpcv1beta1.Code_CODE_OK {
			return false
		}

		scope, err := AddOwnerScope(map[string]*authpb.Scope{})
		if err != nil {
			return false
		}
		token, err := mgr.MintToken(ctx, userResp.User, scope)
		if err != nil {
			return false
		}

		ctx = metadata.AppendToOutgoingContext(context.Background(), ctxpkg.TokenHeader, token)
		refStat, err := client.Stat(ctx, &provider.StatRequest{Ref: r})
		if err != nil || refStat.Status.Code != rpcv1beta1.Code_CODE_OK {
			return false
		}

		return strings.HasPrefix(refStat.Info.Path, shareStat.Info.Path)
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
