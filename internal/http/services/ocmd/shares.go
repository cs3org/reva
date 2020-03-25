// Copyright 2018-2020 CERN
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

package ocmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/rs/zerolog"
)

type share struct {
	ShareWith         string        `json:"shareWith"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	ProviderID        string        `json:"providerId"`
	Owner             string        `json:"owner"`
	Sender            string        `json:"sender"`
	OwnerDisplayName  string        `json:"ownerDisplayName"`
	SenderDisplayName string        `json:"senderDisplayName"`
	ShareType         string        `json:"shareType"`
	ResourceType      string        `json:"resourceType"`
	Protocol          *protocolInfo `json:"protocol"`

	ID        string `json:"id,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type protocolInfo struct {
	Name    string           `json:"name"`
	Options *protocolOptions `json:"options"`
}

type protocolOptions struct {
	SharedSecret string `json:"sharedSecret,omitempty"`
	Permissions  string `json:"permissions,omitempty"`
}

func (s *share) JSON() []byte {
	b, _ := json.MarshalIndent(s, "", "   ")
	return b

}

type sharesHandler struct {
	gatewayAddr string
}

func (h *sharesHandler) init(c *Config) {
	h.gatewayAddr = c.GatewaySvc
}

func (h *sharesHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log := appctx.GetLogger(r.Context())
		shareID := path.Base(r.URL.Path)
		log.Debug().Str("method", r.Method).Str("shareID", shareID).Msg("sharesHandler")

		switch r.Method {
		case http.MethodPost:
			h.createShare(w, r)
		case http.MethodGet:
			if shareID == "/" {
				h.listAllShares(w, r)
			} else {
				h.getShare(w, r, shareID)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (h *sharesHandler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// TODO (ishank011): Check if the user is allowed to share the file once the invitation workflow has been implemented.
	// TODO (ishank011): Also check if the provider is authorized or not.
	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error getting storage grpc client on addr: %v", h.gatewayAddr), err)
		return
	}

	hRes, err := gatewayClient.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc get home request", err)
		return
	}
	prefix := hRes.GetPath()

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		WriteError(w, r, APIErrorInvalidParameter, "missing shareWith", nil)
		return
	}

	userRes, err := gatewayClient.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: shareWith},
	})

	if err != nil {
		WriteError(w, r, APIErrorInvalidParameter, "error searching recipient", err)
		return
	}

	if userRes.Status.Code != rpc.Code_CODE_OK {
		WriteError(w, r, APIErrorNotFound, "user not found", err)
		return
	}

	var permissions conversions.Permissions

	role := r.FormValue("role")
	if role == "" {
		pval := r.FormValue("permissions")
		if pval == "" {
			// by default only allow read permissions / assign viewer role
			role = conversions.RoleViewer
		} else {
			pint, err := strconv.Atoi(pval)
			if err != nil {
				WriteError(w, r, APIErrorInvalidParameter, "permissions must be an integer", err)
				return
			}
			permissions, err = conversions.NewPermissions(pint)
			if err != nil {
				WriteError(w, r, APIErrorInvalidParameter, err.Error(), nil)
				return
			}
			role = conversions.Permissions2Role(permissions)
		}
	}

	var resourcePermissions *provider.ResourcePermissions
	resourcePermissions, err = h.role2CS3Permissions(role)
	if err != nil {
		WriteError(w, r, APIErrorInvalidParameter, "unknown role", err)
		return
	}

	roleMap := map[string]string{"name": role}
	val, err := json.Marshal(roleMap)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "could not encode role", err)
		return
	}

	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(prefix, r.FormValue("path")),
			},
		},
	}

	statRes, err := gatewayClient.Stat(ctx, statReq)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc stat request", err)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			WriteError(w, r, APIErrorNotFound, "not found", nil)
			return
		}
		WriteError(w, r, APIErrorServerError, "grpc stat request failed", err)
		return
	}

	createShareReq := &ocm.CreateOCMShareRequest{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"role": &types.OpaqueEntry{
					Decoder: "json",
					Value:   val,
				},
			},
		},
		ResourceId: statRes.Info.Id,
		Grant: &ocm.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   userRes.User.GetId(),
			},
			Permissions: &ocm.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
	}

	createShareResponse, err := gatewayClient.CreateOCMShare(ctx, createShareReq)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc create share request", err)
		return
	}
	if createShareResponse.Status.Code != rpc.Code_CODE_OK {
		if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			WriteError(w, r, APIErrorNotFound, "not found", nil)
			return
		}
		WriteError(w, r, APIErrorServerError, "grpc create share request failed", err)
		return
	}

	log.Info().Msg("Share created.")
}

func (h *sharesHandler) getShare(w http.ResponseWriter, r *http.Request, shareID string) {
}

func (h *sharesHandler) listAllShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(r.Context())
	user := r.Header.Get("Remote-User")

	log.Debug().Str("ctx", fmt.Sprintf("%+v", ctx)).Str("user", user).Msg("listAllShares")
	log.Debug().Str("Variable: `h` type", fmt.Sprintf("%T", h)).Str("Variable: `h` value", fmt.Sprintf("%+v", h)).Msg("listAllShares")

	shares, err := h.getShares(ctx, log, user)

	log.Debug().Str("err", fmt.Sprintf("%+v", err)).Str("shares", fmt.Sprintf("%+v", shares)).Msg("listAllShares")

	if err != nil {
		log.Err(err).Msg("Error reading shares from manager")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (h *sharesHandler) getShares(ctx context.Context, logger *zerolog.Logger, user string) ([]*share, error) {

	gateway, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		return nil, err
	}

	filters := []*link.ListPublicSharesRequest_Filter{}
	req := link.ListPublicSharesRequest{
		Filters: filters,
	}

	logger.Debug().Str("gateway", fmt.Sprintf("%+v", gateway)).Str("req", fmt.Sprintf("%+v", req)).Msg("GetShares")

	res, err := gateway.ListPublicShares(ctx, &req)

	logger.Debug().Str("response", fmt.Sprintf("%+v", res)).Str("err", fmt.Sprintf("%+v", err)).Msg("GetShares")

	if err != nil {
		return nil, err
	}

	shares := make([]*share, 0)

	for i, publicShare := range res.GetShare() {
		logger.Debug().Str("idx", string(i)).Str("share", fmt.Sprintf("%+v", publicShare)).Msg("GetShares")

		share := convertPublicShareToShare(publicShare)
		shares = append(shares, share)
	}

	logger.Debug().Str("shares", fmt.Sprintf("%+v", shares)).Msg("GetShares")
	return shares, nil
}

func (h *sharesHandler) role2CS3Permissions(r string) (*provider.ResourcePermissions, error) {
	switch r {
	case conversions.RoleViewer:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
		}, nil
	case conversions.RoleEditor:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,
		}, nil
	case conversions.RoleCoowner:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,

			AddGrant:    true,
			RemoveGrant: true, // TODO when are you able to unshare / delete
			UpdateGrant: true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown role: %s", r)
	}
}

func convertPublicShareToShare(publicShare *link.PublicShare) *share {
	return &share{
		ID: publicShare.GetId().String(),
	}
}
