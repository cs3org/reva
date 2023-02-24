package scope

import (
	"context"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

func ocmShareScope(_ context.Context, scope *authpb.Scope, resource interface{}, _ *zerolog.Logger) (bool, error) {
	switch v := resource.(type) {
	// TODO: defined where an ocmshare scope can go
	case string:
		return checkResourcePath(v), nil
	}
	return false, nil
}

// AddOCMShareScope adds the scope to allow access to an OCM share and the share resource.
func AddOCMShareScope(share *ocmv1beta1.Share, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
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
	}
	return scopes, nil
}
