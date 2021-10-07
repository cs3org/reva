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

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	statuspkg "github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/utils"
)

func expandAndVerifyScope(ctx context.Context, req interface{}, tokenScope map[string]*authpb.Scope, gatewayAddr string) error {
	log := appctx.GetLogger(ctx)

	if ref, ok := extractRef(req); ok {
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
					if ok, err := checkResourcePath(ctx, ref, share.ResourceId, gatewayAddr); err == nil && ok {
						return nil
					}

				case strings.HasPrefix(k, "share"):
					var share collaboration.Share
					err := utils.UnmarshalJSONToProtoV1(tokenScope[k].Resource.Value, &share)
					if err != nil {
						continue
					}
					if ok, err := checkResourcePath(ctx, ref, share.ResourceId, gatewayAddr); err == nil && ok {
						return nil
					}
				case strings.HasPrefix(k, "lightweight"):
					client, err := pool.GetGatewayServiceClient(gatewayAddr)
					if err != nil {
						continue
					}
					shares, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
					if err != nil || shares.Status.Code != rpc.Code_CODE_OK {
						log.Warn().Err(err).Msg("error listing received shares")
						continue
					}
					for _, share := range shares.Shares {
						if ok, err := checkResourcePath(ctx, ref, share.Share.ResourceId, gatewayAddr); err == nil && ok {
							return nil
						}
					}
				}
			}
		} else {
			// ref has ID present
			// The request might be coming from a share created for a lightweight account
			// after the token was minted.
			log.Info().Msgf("resolving ID reference against received shares to verify token scope %+v", ref.GetResourceId())
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
					for _, share := range shares.Shares {
						if utils.ResourceIDEqual(share.Share.ResourceId, ref.GetResourceId()) {
							return nil
						}
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

func checkResourcePath(ctx context.Context, ref *provider.Reference, r *provider.ResourceId, gatewayAddr string) (bool, error) {
	client, err := pool.GetGatewayServiceClient(gatewayAddr)
	if err != nil {
		return false, err
	}

	// Since the resource ID is obtained from the scope, the current token
	// has access to it.
	statReq := &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: r},
	}

	statResponse, err := client.Stat(ctx, statReq)
	if err != nil {
		return false, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return false, statuspkg.NewErrorFromCode(statResponse.Status.Code, "auth interceptor")
	}

	if strings.HasPrefix(ref.GetPath(), statResponse.Info.Path) {
		// The path corresponds to the resource to which the token has access.
		// We allow access to it.
		return true, nil
	}
	return false, nil
}

func extractRef(req interface{}) (*provider.Reference, bool) {
	switch v := req.(type) {
	case *registry.GetStorageProvidersRequest:
		return v.GetRef(), true
	case *provider.StatRequest:
		return v.GetRef(), true
	case *provider.ListContainerRequest:
		return v.GetRef(), true
	case *provider.CreateContainerRequest:
		return v.GetRef(), true
	case *provider.DeleteRequest:
		return v.GetRef(), true
	case *provider.MoveRequest:
		return v.GetSource(), true
	case *provider.InitiateFileDownloadRequest:
		return v.GetRef(), true
	case *provider.InitiateFileUploadRequest:
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
