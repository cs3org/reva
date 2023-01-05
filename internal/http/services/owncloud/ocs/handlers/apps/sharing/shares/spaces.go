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

package shares

import (
	"context"
	"fmt"
	"net/http"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

func (h *Handler) getGrantee(ctx context.Context, name string) (provider.Grantee, error) {
	log := appctx.GetLogger(ctx)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		return provider.Grantee{}, err
	}
	userRes, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username",
		Value: name,
	})
	if err == nil && userRes.Status.Code == rpc.Code_CODE_OK {
		return provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: userRes.User.Id},
		}, nil
	}
	log.Debug().Str("name", name).Msg("no user found")

	groupRes, err := client.GetGroupByClaim(ctx, &groupv1beta1.GetGroupByClaimRequest{
		Claim: "group_name",
		Value: name,
	})
	if err == nil && groupRes.Status.Code == rpc.Code_CODE_OK {
		return provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: groupRes.Group.Id},
		}, nil
	}
	log.Debug().Str("name", name).Msg("no group found")

	return provider.Grantee{}, fmt.Errorf("no grantee found with name %s", name)
}

func (h *Handler) addSpaceMember(w http.ResponseWriter, r *http.Request, info *provider.ResourceInfo, role *conversions.Role, roleVal []byte) {
	ctx := r.Context()

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith", nil)
		return
	}

	grantee, err := h.getGrantee(ctx, shareWith)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting grantee", err)
		return
	}

	ref := &provider.Reference{ResourceId: info.Id}

	providers, err := h.findProviders(ctx, ref)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting storage provider", err)
		return
	}

	providerClient, err := h.getStorageProviderClient(providers[0])
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting storage provider", err)
		return
	}

	addGrantRes, err := providerClient.AddGrant(ctx, &provider.AddGrantRequest{
		Ref: ref,
		Grant: &provider.Grant{
			Grantee:     &grantee,
			Permissions: role.CS3ResourcePermissions(),
		},
	})
	if err != nil || addGrantRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "could not add space member", err)
		return
	}

	response.WriteOCSSuccess(w, r, nil)
}

func (h *Handler) removeSpaceMember(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()

	shareWith := r.URL.Query().Get("shareWith")
	if shareWith == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith", nil)
		return
	}

	grantee, err := h.getGrantee(ctx, shareWith)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting grantee", err)
		return
	}

	ref, err := utils.ParseStorageSpaceReference(spaceID)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "could not parse space id", err)
		return
	}

	providers, err := h.findProviders(ctx, &ref)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting storage provider", err)
		return
	}

	providerClient, err := h.getStorageProviderClient(providers[0])
	if err != nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "error getting storage provider", err)
		return
	}

	removeGrantRes, err := providerClient.RemoveGrant(ctx, &provider.RemoveGrantRequest{
		Ref: &ref,
		Grant: &provider.Grant{
			Grantee: &grantee,
		},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error removing grant", err)
		return
	}
	if removeGrantRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error removing grant", err)
		return
	}

	response.WriteOCSSuccess(w, r, nil)
}

func (h *Handler) getStorageProviderClient(p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(pool.Endpoint(p.Address))
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (h *Handler) findProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
	c, err := pool.GetStorageRegistryClient(pool.Endpoint(h.storageRegistryAddr))
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	res, err := c.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
		Ref: ref,
	})

	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetStorageProvider")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + ref.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		default:
			return nil, status.NewErrorFromCode(res.Status.Code, "gateway")
		}
	}

	if res.Providers == nil {
		return nil, errtypes.NotFound("gateway: provider is nil")
	}

	return res.Providers, nil
}
