package scope

import (
	"context"
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
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

// FIXME: the namespace here is hardcoded
// find a way to pass it from the config
const ocmNamespace = "/ocm"

func ocmShareScope(_ context.Context, scope *authpb.Scope, resource interface{}, _ *zerolog.Logger) (bool, error) {
	var share ocmv1beta1.Share
	if err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &share); err != nil {
		return false, err
	}

	switch v := resource.(type) {

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

	case *provider.CreateContainerRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.TouchFileRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.DeleteRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.MoveRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetSource(), ocmNamespace) && checkStorageRefForOCMShare(&share, v.GetDestination(), ocmNamespace), nil
	case *provider.InitiateFileUploadRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.SetArbitraryMetadataRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil
	case *provider.UnsetArbitraryMetadataRequest:
		return hasRoleEditor(*scope) && checkStorageRefForOCMShare(&share, v.GetRef(), ocmNamespace), nil

	// App provider requests
	case *appregistry.GetDefaultAppProviderForMimeTypeRequest:
		return true, nil

	case *userv1beta1.GetUserByClaimRequest:
		return true, nil

	case *ocmv1beta1.GetOCMShareRequest:
		return checkOCMShareRef(&share, v.GetRef()), nil
	case string:
		return checkResourcePath(v), nil
	}
	return false, nil
}

func checkStorageRefForOCMShare(s *ocmv1beta1.Share, r *provider.Reference, ns string) bool {
	if r.ResourceId != nil {
		return utils.ResourceIDEqual(s.ResourceId, r.GetResourceId()) || strings.HasPrefix(r.ResourceId.OpaqueId, s.Token)
	}

	// FIXME: the path here is hardcoded
	return strings.HasPrefix(r.GetPath(), filepath.Join(ns, s.Token))
}

func checkOCMShareRef(s *ocmv1beta1.Share, ref *ocmv1beta1.ShareReference) bool {
	return ref.GetToken() == s.Token
}

// AddOCMShareScope adds the scope to allow access to an OCM share and the share resource.
func AddOCMShareScope(share *ocmv1beta1.Share, role authpb.Role, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	// Create a new "scope share" to only expose the required fields `ResourceId` and `Token` to the scope.
	scopeShare := ocmv1beta1.Share{ResourceId: share.ResourceId, Token: share.Token}
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
