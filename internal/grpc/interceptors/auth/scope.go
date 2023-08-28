// Copyright 2018-2023 CERN
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
	"path/filepath"
	"strings"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
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
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"google.golang.org/grpc/metadata"
)

const (
	scopeDelimiter       = "#"
	scopeCacheExpiration = 3600
)

func expandAndVerifyScope(ctx context.Context, req interface{}, tokenScope map[string]*authpb.Scope, user *userpb.User, gatewayAddr string, mgr token.Manager) error {
	log := appctx.GetLogger(ctx)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(gatewayAddr))
	if err != nil {
		return err
	}
	log.Trace().Msg("Extracting scope from token")
	if ref, ok := extractRef(req, tokenScope); ok {
		// The request is for a storage reference. This can be the case for multiple scenarios:
		// - If the path is not empty, the request might be coming from a share where the accessor is
		//   trying to impersonate the owner, since the share manager doesn't know the
		//   share path.
		// - If the ID not empty, the request might be coming from
		//   - a resource present inside a shared folder, or
		//   - a share created for a lightweight account after the token was minted.
		log.Info().Msgf("resolving storage reference to check token scope %s", ref.String())
		for k := range tokenScope {
			switch {
			case strings.HasPrefix(k, "publicshare"):
				if err = resolvePublicShare(ctx, ref, tokenScope[k], client, mgr); err == nil {
					return nil
				}
			case strings.HasPrefix(k, "share"):
				if err = resolveUserShare(ctx, ref, tokenScope[k], client, mgr); err == nil {
					return nil
				}
			case strings.HasPrefix(k, "ocmshare"):
				if err = resolveOCMShare(ctx, ref, tokenScope[k], client, mgr); err == nil {
					return nil
				}
			}
			if err != nil {
				log.Err(err).Msgf("error resolving reference %s under scope %+v", ref.String(), k)
			}
		}
	} else {
		log.Info().Msg("Token scope is not ok")
	}
	log.Info().Msg("Done extracting scope from token")

	if checkLightweightScope(ctx, req, tokenScope, client) {
		return nil
	}

	return errtypes.PermissionDenied("access to resource not allowed within the assigned scope")
}

func hasLightweightScope(tokenScope map[string]*authpb.Scope) bool {
	for scope := range tokenScope {
		if strings.HasPrefix(scope, "lightweight") {
			return true
		}
	}
	return false
}

func checkLightweightScope(ctx context.Context, req interface{}, tokenScope map[string]*authpb.Scope, client gateway.GatewayAPIClient) bool {
	if !hasLightweightScope(tokenScope) {
		return false
	}

	switch r := req.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return true
	case *provider.StatRequest:
		return true
	case *appregistry.GetAppProvidersRequest:
		return true
	case *provider.ListContainerRequest:
		return hasPermissions(ctx, client, r.GetRef(), &provider.ResourcePermissions{
			ListContainer: true,
		})
	case *provider.InitiateFileDownloadRequest:
		return hasPermissions(ctx, client, r.GetRef(), &provider.ResourcePermissions{
			InitiateFileDownload: true,
		})
	case *appprovider.OpenInAppRequest:
		return hasPermissions(ctx, client, &provider.Reference{ResourceId: r.ResourceInfo.Id}, &provider.ResourcePermissions{
			InitiateFileDownload: true,
		})
	case *gateway.OpenInAppRequest:
		return hasPermissions(ctx, client, r.GetRef(), &provider.ResourcePermissions{
			InitiateFileDownload: true,
		})

	// Editor role
	case *provider.CreateContainerRequest:
		parent, err := parentOfResource(ctx, client, r.GetRef())
		if err != nil {
			return false
		}
		return hasPermissions(ctx, client, parent, &provider.ResourcePermissions{
			CreateContainer: true,
		})
	case *provider.TouchFileRequest:
		parent, err := parentOfResource(ctx, client, r.GetRef())
		if err != nil {
			return false
		}
		return hasPermissions(ctx, client, parent, &provider.ResourcePermissions{
			InitiateFileUpload: true,
		})
	case *provider.DeleteRequest:
		return hasPermissions(ctx, client, r.GetRef(), &provider.ResourcePermissions{
			Delete: true,
		})
	case *provider.MoveRequest:
		return hasPermissions(ctx, client, r.Source, &provider.ResourcePermissions{
			InitiateFileDownload: true,
		}) && hasPermissions(ctx, client, r.Destination, &provider.ResourcePermissions{
			InitiateFileUpload: true,
		})
	case *provider.InitiateFileUploadRequest:
		if hasPermissions(ctx, client, r.GetRef(), &provider.ResourcePermissions{
			InitiateFileUpload: true,
		}) {
			return true
		}
		parent, err := parentOfResource(ctx, client, r.GetRef())
		if err != nil {
			return false
		}
		return hasPermissions(ctx, client, parent, &provider.ResourcePermissions{
			InitiateFileUpload: true,
		})
	}

	return false
}

func parentOfResource(ctx context.Context, client gateway.GatewayAPIClient, ref *provider.Reference) (*provider.Reference, error) {
	if utils.IsAbsolutePathReference(ref) {
		parent := filepath.Dir(ref.GetPath())
		info, err := stat(ctx, client, &provider.Reference{Path: parent})
		if err != nil {
			return nil, err
		}
		return &provider.Reference{ResourceId: info.Id}, nil
	}

	info, err := stat(ctx, client, ref)
	if err != nil {
		return nil, err
	}
	return &provider.Reference{ResourceId: info.ParentId}, nil
}

func stat(ctx context.Context, client gateway.GatewayAPIClient, ref *provider.Reference) (*provider.ResourceInfo, error) {
	statRes, err := client.Stat(ctx, &provider.StatRequest{
		Ref: ref,
	})

	switch {
	case err != nil:
		return nil, err
	case statRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(statRes.Status.Message)
	case statRes.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(statRes.Status.Message)
	}

	return statRes.Info, nil
}

func hasPermissions(ctx context.Context, client gateway.GatewayAPIClient, ref *provider.Reference, permissionSet *provider.ResourcePermissions) bool {
	info, err := stat(ctx, client, ref)
	if err != nil {
		return false
	}
	return utils.HasPermissions(info.PermissionSet, permissionSet)
}

func resolvePublicShare(ctx context.Context, ref *provider.Reference, scope *authpb.Scope, client gateway.GatewayAPIClient, mgr token.Manager) error {
	var share link.PublicShare
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share)
	if err != nil {
		return err
	}

	return checkCacheForNestedResource(ctx, ref, share.ResourceId, client, mgr)
}

func resolveOCMShare(ctx context.Context, ref *provider.Reference, scope *authpb.Scope, client gateway.GatewayAPIClient, mgr token.Manager) error {
	var share ocmv1beta1.Share
	if err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share); err != nil {
		return err
	}

	return checkCacheForNestedResource(ctx, ref, share.ResourceId, client, mgr)
}

func resolveUserShare(ctx context.Context, ref *provider.Reference, scope *authpb.Scope, client gateway.GatewayAPIClient, mgr token.Manager) error {
	var share collaboration.Share
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share)
	if err != nil {
		return err
	}

	return checkCacheForNestedResource(ctx, ref, share.ResourceId, client, mgr)
}

func checkCacheForNestedResource(ctx context.Context, ref *provider.Reference, resource *provider.ResourceId, client gateway.GatewayAPIClient, mgr token.Manager) error {
	// Check if this ref is cached
	key := resourceid.OwnCloudResourceIDWrap(resource) + scopeDelimiter + getRefKey(ref)
	if _, err := scopeExpansionCache.Get(key); err == nil {
		return nil
	}

	if ok, err := checkIfNestedResource(ctx, ref, resource, client, mgr); err == nil && ok {
		_ = scopeExpansionCache.SetWithExpire(key, nil, scopeCacheExpiration*time.Second)
		return nil
	}

	return errtypes.PermissionDenied("request is not for a nested resource")
}

func isRelativePathOrEmpty(path string) bool {
	if len(path) == 0 {
		return true
	}
	return path[0] != '/'
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
	if isRelativePathOrEmpty(childPath) {
		// We mint a token as the owner of the public share and try to stat the reference
		// TODO(ishank011): We need to find a better alternative to this

		userResp, err := client.GetUser(ctx, &userpb.GetUserRequest{UserId: statResponse.Info.Owner, SkipFetchingUserGroups: true})
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
}

func extractRefForReaderRole(req interface{}) (*provider.Reference, bool) {
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
	case *provider.GetLockRequest:
		return v.GetRef(), true

	// App provider requests
	case *appregistry.GetAppProvidersRequest:
		return &provider.Reference{ResourceId: v.ResourceInfo.Id}, true
	}

	return nil, false
}

func extractRefForUploaderRole(req interface{}) (*provider.Reference, bool) {
	switch v := req.(type) {
	// Write Requests
	case *registry.GetStorageProvidersRequest:
		return v.GetRef(), true
	case *provider.StatRequest:
		return v.GetRef(), true
	case *provider.CreateContainerRequest:
		return v.GetRef(), true
	case *provider.TouchFileRequest:
		return v.GetRef(), true
	case *provider.InitiateFileUploadRequest:
		return v.GetRef(), true
	}

	return nil, false
}

func extractRefForEditorRole(req interface{}) (*provider.Reference, bool) {
	switch v := req.(type) {
	// Remaining edit Requests
	case *provider.DeleteRequest:
		return v.GetRef(), true
	case *provider.MoveRequest:
		return v.GetSource(), true
	case *provider.SetArbitraryMetadataRequest:
		return v.GetRef(), true
	case *provider.UnsetArbitraryMetadataRequest:
		return v.GetRef(), true
	case *provider.SetLockRequest:
		return v.GetRef(), true
	case *provider.RefreshLockRequest:
		return v.GetRef(), true
	case *provider.UnlockRequest:
		return v.GetRef(), true
	}

	return nil, false
}

func extractRef(req interface{}, tokenScope map[string]*authpb.Scope) (*provider.Reference, bool) {
	var readPerm, uploadPerm, editPerm bool
	for _, v := range tokenScope {
		if v.Role == authpb.Role_ROLE_OWNER || v.Role == authpb.Role_ROLE_EDITOR || v.Role == authpb.Role_ROLE_VIEWER {
			readPerm = true
		}
		if v.Role == authpb.Role_ROLE_OWNER || v.Role == authpb.Role_ROLE_EDITOR || v.Role == authpb.Role_ROLE_UPLOADER {
			uploadPerm = true
		}
		if v.Role == authpb.Role_ROLE_OWNER || v.Role == authpb.Role_ROLE_EDITOR {
			editPerm = true
		}
	}

	if readPerm {
		ref, ok := extractRefForReaderRole(req)
		if ok {
			return ref, true
		}
	}
	if uploadPerm {
		ref, ok := extractRefForUploaderRole(req)
		if ok {
			return ref, true
		}
	}
	if editPerm {
		ref, ok := extractRefForEditorRole(req)
		if ok {
			return ref, true
		}
	}

	return nil, false
}

func getRefKey(ref *provider.Reference) string {
	if ref.Path != "" {
		return ref.Path
	}
	return resourceid.OwnCloudResourceIDWrap(ref.ResourceId)
}
