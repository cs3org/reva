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

package scope

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/rs/zerolog"
)

// FIXME: the namespace here is hardcoded
// find a way to pass it from the config.
const ocmNamespace = "/ocm"

func ocmShareScope(_ context.Context, scope *authpb.Scope, resource any, _ *zerolog.Logger) (bool, error) {
	var share ocmv1beta1.Share
	if err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share); err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// viewer role
	case *registry.GetStorageProvidersRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.StatRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.ListContainerRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.InitiateFileDownloadRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *appprovider.OpenInAppRequest:
		return checkStorageRefForOCMShare(&share, &provider.Reference{ResourceId: v.ResourceInfo.Id}, ocmNamespace), nil
	case *gateway.OpenInAppRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.GetLockRequest:
		return checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil

	// editor role
	case *provider.CreateContainerRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.TouchFileRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.DeleteRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.MoveRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetSource(), ocmNamespace) && checkStorageRefForOCMShare(&share, v.GetDestination(), ocmNamespace), nil
	case *provider.InitiateFileUploadRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.SetArbitraryMetadataRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.UnsetArbitraryMetadataRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.SetLockRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.RefreshLockRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.UnlockRequest:
		return hasRoleEditor(scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil

	// App provider requests
	case *appregistry.GetDefaultAppProviderForMimeTypeRequest:
		return true, nil

	case *userv1beta1.GetUserByClaimRequest:
		return true, nil

	case *ocmv1beta1.GetOCMShareRequest:
		return checkOCMShareRef(&share, v.GetRef()), nil
	case string:
		if checkResourcePath(v) {
			return true, nil
		}
		return checkDAVOCMPath(v, &share), nil
	}
	return false, nil
}

// pathUnderOCMPrefix reports whether path is the namespace prefix or a child under it.
// We require a path segment after the prefix (path == prefix or path starts with prefix+"/")
// so that names that merely start with the prefix string do not match. Example: /ocm-m6-proof.txt
// must not match prefix /ocm; only /ocm or /ocm/... may match.
func pathUnderOCMPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// checkStorageRefForOCMShare reports whether ref refers to a resource inside this OCM share.
// Flow: ref is either by ResourceId (storage opaque id) or by path. We allow only when the
// ref clearly belongs to this share. Used at gRPC (storage) scope check; for path refs at
// space root we get ref.Path = "/<filename>", so pathUnderOCMPrefix avoids matching "/ocm-*.txt".
func checkStorageRefForOCMShare(s *ocmv1beta1.Share, r *provider.Reference, ns string) bool {
	shareID := s.Id.GetOpaqueId()

	// Ref by ResourceId: allow only if ref equals share resource, or ref id is scoped to this share.
	if r.ResourceId != nil {
		if utils.ResourceIDEqual(s.ResourceId, r.GetResourceId()) {
			return true
		}
		if shareID != "" && r.ResourceId.OpaqueId == shareID {
			return true
		}
		// Only when share has a token. Code-flow scope omits token; matching
		// against an empty string would be meaningless, so skip when unset.
		if s.Token != "" && r.ResourceId.OpaqueId == s.Token {
			return true
		}
		return false
	}

	// Ref by path: allow only if path is under this share in the OCM namespace (ns/shareID or ns/token).
	path := r.GetPath()
	underShareID := shareID != "" && pathUnderOCMPrefix(path, filepath.Join(ns, shareID))
	underToken := s.Token != "" && pathUnderOCMPrefix(path, filepath.Join(ns, s.Token))
	return underShareID || underToken
}

// checkDAVOCMPath authorizes HTTP path strings for OCM DAV requests scoped to
// this share. the HTTP auth interceptor's VerifyScope call dispatches here for
// DAV paths that checkResourcePath does not cover. Only /dav/ocm/... and
// /remote.php/dav/ocm/... are accepted; non-DAV /ocm/* routes are rejected.
func checkDAVOCMPath(rawPath string, s *ocmv1beta1.Share) bool {
	cleaned := path.Clean(rawPath)
	segments := strings.Split(cleaned, "/")
	// path.Clean preserves the leading "/", so segments[0] is always "".
	segments = segments[1:]

	// Strip optional leading "remote.php" deployment wrapper.
	if len(segments) > 0 && segments[0] == "remote.php" {
		segments = segments[1:]
	}

	// Require exactly "dav", "ocm" as the next two segments.
	if len(segments) < 2 || segments[0] != "dav" || segments[1] != "ocm" {
		return false
	}

	// Bare-root PROPFIND at the OCM mount point: /dav/ocm (no segment after ocm).
	if len(segments) == 2 {
		return true
	}

	// The segment after "ocm" must match the share's canonical ID or legacy token.
	target := segments[2]
	shareID := s.Id.GetOpaqueId()
	if shareID != "" && target == shareID {
		return true
	}
	if s.Token != "" && target == s.Token {
		return true
	}

	return false
}

func checkOCMShareRef(s *ocmv1beta1.Share, ref *ocmv1beta1.ShareReference) bool {
	if id := ref.GetId(); id != nil {
		return id.GetOpaqueId() == s.Id.GetOpaqueId()
	}
	return ref.GetToken() == s.Token
}

// AddOCMShareScope adds the scope to allow access to an OCM share and the share resource.
// It carries the share metadata needed to resolve authenticated DAV requests without a second
// repository lookup, including Token for backward compatibility with legacy direct-secret flows.
func AddOCMShareScope(share *ocmv1beta1.Share, role authpb.Role, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	scopeShare := ocmv1beta1.Share{
		ResourceId:    share.ResourceId,
		Id:            share.Id,
		Token:         share.Token,
		Creator:       share.Creator,
		AccessMethods: share.AccessMethods,
	}
	val, err := utils.MarshalProtoV1ToJSON(&scopeShare)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}

	scopes["ocmshare:"+share.Id.OpaqueId] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: role,
	}
	return scopes, nil
}

// AddCodeFlowOCMShareScope adds a shareId/resource-only scope used by code-flow exchanged JWTs.
// Unlike AddOCMShareScope, it deliberately omits Token so the long-lived shared secret
// is never embedded in exchanged-token scopes.
func AddCodeFlowOCMShareScope(share *ocmv1beta1.Share, role authpb.Role, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	scopeShare := ocmv1beta1.Share{
		ResourceId:    share.ResourceId,
		Id:            share.Id,
		Creator:       share.Creator,
		AccessMethods: share.AccessMethods,
	}
	val, err := utils.MarshalProtoV1ToJSON(&scopeShare)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}

	scopes["ocmshare:"+share.Id.OpaqueId] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: role,
	}
	return scopes, nil
}

// GetOCMSharesFromScopes returns all OCM shares in the given scope.
func GetOCMSharesFromScopes(scopes map[string]*authpb.Scope) ([]*ocmv1beta1.Share, error) {
	var shares []*ocmv1beta1.Share
	for k, s := range scopes {
		if strings.HasPrefix(k, "ocmshare:") {
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
	}
	return shares, nil
}
