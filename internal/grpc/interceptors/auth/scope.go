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

package auth

import (
	"context"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	statuspkg "github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/utils"
	"google.golang.org/grpc/metadata"
)

func expandAndVerifyScope(ctx context.Context, req interface{}, tokenScope map[string]*authpb.Scope, gatewayAddr string, mgr token.Manager) error {
	log := appctx.GetLogger(ctx)
	client, err := pool.GetGatewayServiceClient(gatewayAddr)
	if err != nil {
		return err
	}

	hasEditorRole := false
	for _, v := range tokenScope {
		if v.Role == authpb.Role_ROLE_EDITOR {
			hasEditorRole = true
		}
	}

	if ref, ok := extractRef(req, hasEditorRole); ok {
		// Check if req is of type *provider.Reference_Path
		// If yes, the request might be coming from a share where the accessor is
		// trying to impersonate the owner, since the share manager doesn't know the
		// share path.
		if ref.GetPath() != "" {
			log.Info().Msgf("resolving path reference to ID to check token scope %+v", ref.GetPath())
			for k := range tokenScope {
				switch {
				case strings.HasPrefix(k, "publicshare"):
					var share link.PublicShare
					err := utils.UnmarshalJSONToProtoV1(tokenScope[k].Resource.Value, &share)
					if err != nil {
						continue
					}
					if ok, err := checkIfNestedResource(ctx, ref, share.ResourceId, client, mgr); err == nil && ok {
						return nil
					}

				case strings.HasPrefix(k, "share"):
					var share collaboration.Share
					err := utils.UnmarshalJSONToProtoV1(tokenScope[k].Resource.Value, &share)
					if err != nil {
						continue
					}
					if ok, err := checkIfNestedResource(ctx, ref, share.ResourceId, client, mgr); err == nil && ok {
						return nil
					}
				case strings.HasPrefix(k, "lightweight"):
					shares, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
					if err != nil || shares.Status.Code != rpc.Code_CODE_OK {
						log.Warn().Err(err).Msg("error listing received shares")
						continue
					}
					for _, share := range shares.Shares {
						if ok, err := checkIfNestedResource(ctx, ref, share.Share.ResourceId, client, mgr); err == nil && ok {
							return nil
						}
					}
				}
			}
		} else {
			// ref has ID present
			// The request might be coming from
			// - a resource present inside a shared folder, or
			// - a share created for a lightweight account after the token was minted.

			client, err := pool.GetGatewayServiceClient(gatewayAddr)
			if err != nil {
				return err
			}
			for k := range tokenScope {
				if strings.HasPrefix(k, "lightweight") {
					log.Info().Msgf("resolving ID reference against received shares to verify token scope %+v", ref.GetResourceId())
					shares, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
					if err != nil || shares.Status.Code != rpc.Code_CODE_OK {
						log.Warn().Err(err).Msg("error listing received shares")
						continue
					}
					for _, share := range shares.Shares {
						if utils.ResourceIDEqual(share.Share.ResourceId, ref.GetResourceId()) {
							return nil
						}
					}
				} else if strings.HasPrefix(k, "publicshare") {
					var share link.PublicShare
					err := utils.UnmarshalJSONToProtoV1(tokenScope[k].Resource.Value, &share)
					if err != nil {
						continue
					}
					if ok, err := checkIfNestedResource(ctx, ref, share.ResourceId, client, mgr); err == nil && ok {
						return nil
					}
				}
			}
		}

	} else if ref, ok := extractShareRef(req); ok {
		// It's a share ref
		// The request might be coming from a share created for a lightweight account
		// after the token was minted.
		log.Info().Msgf("resolving share reference against received shares to verify token scope %+v", ref)
		client, err := pool.GetGatewayServiceClient(gatewayAddr)
		if err != nil {
			return err
		}
		for k := range tokenScope {
			if strings.HasPrefix(k, "lightweight") {
				shares, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
				if err != nil || shares.Status.Code != rpc.Code_CODE_OK {
					log.Warn().Err(err).Msg("error listing received shares")
					continue
				}
				for _, s := range shares.Shares {
					if ref.GetId() != nil && ref.GetId().OpaqueId == s.Share.Id.OpaqueId {
						return nil
					}
					if key := ref.GetKey(); key != nil && (utils.UserEqual(key.Owner, s.Share.Owner) || utils.UserEqual(key.Owner, s.Share.Creator)) &&
						utils.ResourceIDEqual(key.ResourceId, s.Share.ResourceId) && utils.GranteeEqual(key.Grantee, s.Share.Grantee) {
						return nil
					}
				}
			}
		}
	}

	return errtypes.PermissionDenied("access to resource not allowed within the assigned scope")
}

func checkIfNestedResource(ctx context.Context, ref *provider.Reference, parent *provider.ResourceId, client gateway.GatewayAPIClient, mgr token.Manager) (bool, error) {
	// Since the resource ID is obtained from the scope, the current token
	// has access to it.
	statResponse, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: parent}})
	if err != nil {
		return false, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return false, statuspkg.NewErrorFromCode(statResponse.Status.Code, "auth interceptor")
	}
	parentPath := statResponse.Info.Path

	childPath := ref.GetPath()
	if childPath == "" {
		// We mint a token as the owner of the public share and try to stat the reference
		// TODO(ishank011): We need to find a better alternative to this

		userResp, err := client.GetUser(ctx, &userpb.GetUserRequest{UserId: statResponse.Info.Owner})
		if err != nil || userResp.Status.Code != rpc.Code_CODE_OK {
			return false, err
		}

		scope, err := scope.AddOwnerScope(map[string]*authpb.Scope{})
		if err != nil {
			return false, err
		}
		token, err := mgr.MintToken(ctx, userResp.User, scope)
		if err != nil {
			return false, err
		}
		ctx = metadata.AppendToOutgoingContext(context.Background(), ctxpkg.TokenHeader, token)

		childStat, err := client.Stat(ctx, &provider.StatRequest{Ref: ref})
		if err != nil {
			return false, err
		}
		if childStat.Status.Code != rpc.Code_CODE_OK {
			return false, statuspkg.NewErrorFromCode(childStat.Status.Code, "auth interceptor")
		}
		childPath = statResponse.Info.Path
	}

	return strings.HasPrefix(childPath, parentPath), nil

	// resourcePath := statResponse.Info.Path

	// if strings.HasPrefix(ref.GetPath(), resourcePath) {
	// 	// The path corresponds to the resource to which the token has access.
	// 	// We allow access to it.
	// 	return true, nil
	// }

	// // If we arrived here that could mean that ref.GetPath is not prefixed with the storage mount path but resourcePath is
	// // because it was returned by the gateway which will prefix it. To fix that we remove the mount path from the resourcePath.
	// // resourcePath = "/users/<name>/some/path"
	// // After the split we have [" ", "users", "<name>/some/path"].
	// trimmedPath := "/" + strings.SplitN(resourcePath, "/", 3)[2]
	// if strings.HasPrefix(ref.GetPath(), trimmedPath) {
	// 	// The path corresponds to the resource to which the token has access.
	// 	// We allow access to it.
	// 	return true, nil
	// }

	// return false, nil
}

func extractRef(req interface{}, hasEditorRole bool) (*provider.Reference, bool) {
	switch v := req.(type) {
	// Read requests
	case *registry.GetStorageProvidersRequest:
		return v.GetRef(), true
	case *provider.StatRequest:
		return v.GetRef(), true
	case *provider.ListContainerRequest:
		return v.GetRef(), true
	case *provider.InitiateFileDownloadRequest:
		return v.GetRef(), true
	case *appprovider.OpenInAppRequest:
		return &provider.Reference{ResourceId: v.ResourceInfo.Id}, true
	case *gateway.OpenInAppRequest:
		return v.GetRef(), true

		// App provider requests
	case *appregistry.GetAppProvidersRequest:
		return &provider.Reference{ResourceId: v.ResourceInfo.Id}, true
	}

	if !hasEditorRole {
		return nil, false
	}

	switch v := req.(type) {
	// Write Requests
	case *provider.CreateContainerRequest:
		return v.GetRef(), true
	case *provider.DeleteRequest:
		return v.GetRef(), true
	case *provider.MoveRequest:
		return v.GetSource(), true
	case *provider.InitiateFileUploadRequest:
		return v.GetRef(), true
	case *provider.SetArbitraryMetadataRequest:
		return v.GetRef(), true
	case *provider.UnsetArbitraryMetadataRequest:
		return v.GetRef(), true

	}
	return nil, false
}

func extractShareRef(req interface{}) (*collaboration.ShareReference, bool) {
	switch v := req.(type) {
	case *collaboration.GetReceivedShareRequest:
		return v.GetRef(), true
	case *collaboration.UpdateReceivedShareRequest:
		return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: v.GetShare().GetShare().GetId()}}, true
	}
	return nil, false
}
