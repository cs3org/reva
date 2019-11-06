// Copyright 2018-2019 CERN
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

package ocs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
)

// SharesHandler implements the ownCloud sharing API
type SharesHandler struct {
	gatewayAddr string
	r           *http.Request
	w           http.ResponseWriter
}

func (h *SharesHandler) init(c *Config) error {
	h.gatewayAddr = c.GatewaySvc
	return nil
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = rhttp.ShiftPath(r.URL.Path)

	// TODO(refs) move this away from here
	h.r = r
	h.w = w

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		switch r.Method {
		case "OPTIONS":
			w.WriteHeader(http.StatusOK) // TODO cors?
			return
		case "GET":
			h.listShares()
		case "POST":
			h.createShare(w, r)
		case "PUT":
			// TODO PUT is used with incomplete data to update a share 🤦
			h.updateShare(w, r)
		default:
			WriteOCSError(w, r, MetaBadRequest.StatusCode, "Only GET, POST and PUT are allowed", nil)
		}
	case "sharees":
		h.findSharees(w, r)
	default:
		WriteOCSError(w, r, MetaNotFound.StatusCode, "Not found", nil)
	}
}

func (h *SharesHandler) findSharees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	search := r.URL.Query().Get("search")

	if search == "" {
		WriteOCSError(w, r, MetaBadRequest.StatusCode, "search must not be empty", nil)
		return
	}

	gatewayProvider, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error connecting to user provider", err)
		return
	}

	req := userproviderv0alphapb.FindUsersRequest{
		Filter: search,
	}

	res, err := gatewayProvider.FindUsers(ctx, &req)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error searching users", err)
		return
	}

	log.Debug().Int("count", len(res.GetUsers())).Str("search", search).Msg("users found")

	matches := make([]*conversions.MatchData, 0, len(res.GetUsers()))

	for _, user := range res.GetUsers() {
		match := h.userAsMatch(user)
		log.Debug().Interface("user", user).Interface("match", match).Msg("mapped")
		matches = append(matches, match)
	}

	WriteOCSSuccess(w, r, &conversions.ShareeData{
		Exact: &conversions.ExactMatchesData{
			Users:   []*conversions.MatchData{},
			Groups:  []*conversions.MatchData{},
			Remotes: []*conversions.MatchData{},
		},
		Users:   matches,
		Groups:  []*conversions.MatchData{},
		Remotes: []*conversions.MatchData{},
	})
}

func (h *SharesHandler) userAsMatch(u *userproviderv0alphapb.User) *conversions.MatchData {
	return &conversions.MatchData{
		Label: u.DisplayName,
		Value: &conversions.MatchValueData{
			ShareType: int(conversions.ShareTypeUser),
			// TODO(jfd) find more robust userid
			// username might be ok as it is uniqe at a given point in time
			ShareWith: u.Username,
		},
	}
}

func (h *SharesHandler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		WriteOCSError(w, r, MetaBadRequest.StatusCode, "shareType must be an integer", nil)
		return
	}

	if shareType == int(conversions.ShareTypeUser) {

		// if user sharing is disabled
		if h.gatewayAddr == "" {
			WriteOCSError(w, r, MetaServerError.StatusCode, "user sharing service not configured", nil)
			return
		}

		shareWith := r.FormValue("shareWith")
		if shareWith == "" {
			WriteOCSError(w, r, MetaBadRequest.StatusCode, "missing shareWith", nil)
			return
		}

		gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, fmt.Sprintf("no gateway service on addr: %v", h.gatewayAddr), err)
			return
		}

		res, err := gatewayClient.FindUsers(ctx, &userproviderv0alphapb.FindUsersRequest{
			Filter: shareWith,
		})
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error searching recipient", err)
			return
		}

		var recipient *userproviderv0alphapb.User
		for _, user := range res.GetUsers() {
			if user.Username == shareWith {
				recipient = user
				break
			}
		}

		var pint int

		role := r.FormValue("role")
		// 2. if we don't have a role try to map the permissions
		if role == "" {
			pval := r.FormValue("permissions")
			if pval == "" {
				// by default only allow read permissions / assign viewer role
				role = conversions.RoleViewer
			} else {
				pint, err = strconv.Atoi(pval)
				if err != nil {
					WriteOCSError(w, r, MetaBadRequest.StatusCode, "permissions must be an integer", nil)
					return
				}
				role = h.permissions2Role(pint)
			}
		}

		var permissions *storageproviderv0alphapb.ResourcePermissions
		permissions, err = h.role2CS3Permissions(role)
		if err != nil {
			log.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
			permissions = asCS3Permissions(pint, nil)
		}

		roleMap := map[string]string{"name": role}
		val, err := json.Marshal(roleMap)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "could not encode role", err)
			return
		}

		// TODO(refs) do we need to append the user's home path? path contains only the file path
		statReq := &storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					// Path: "/home" + p,
					Path: "/home/shared.txt", // TODO(refs) remove this hardcoded url. /home/file.ext works whereas /home/username/file.ext crashes.
				},
			},
		}

		sClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error getting storage grpc client", err)
			return
		}

		statRes, err := sClient.Stat(ctx, statReq)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc stat request", err)
			return
		}

		if statRes.Status.Code != rpcpb.Code_CODE_OK {
			if statRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
				return
			}
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc stat request failed", err)
			return
		}

		createShareReq := &usershareproviderv0alphapb.CreateShareRequest{
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"role": &typespb.OpaqueEntry{
						Decoder: "json",
						Value:   val,
					},
				},
			},
			ResourceInfo: statRes.Info,
			Grant: &usershareproviderv0alphapb.ShareGrant{
				Grantee: &storageproviderv0alphapb.Grantee{
					Type: storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER,
					Id:   recipient.Id,
				},
				Permissions: &usershareproviderv0alphapb.SharePermissions{
					Permissions: permissions,
				},
			},
		}

		createShareResponse, err := gatewayClient.CreateShare(ctx, createShareReq)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc create share request", err)
			return
		}
		if createShareResponse.Status.Code != rpcpb.Code_CODE_OK {
			if createShareResponse.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
				return
			}
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc create share request failed", err)
			return
		}
		s, err := h.userShare2ShareData(ctx, createShareResponse.Share)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error mapping share data", err)
			return
		}
		s.Path = r.FormValue("path") // use path without user prefix
		WriteOCSSuccess(w, r, s)
		return
	}

	if shareType == int(conversions.ShareTypePublicLink) {
		// create a public link share
		// get a connection to the public shares service
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msg("error creating public link share")
			WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("error getting a connection to a public shares provider"))
			return
		}

		// u, ok := user.ContextGetUser(ctx)
		// if !ok {
		// 	WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		// 	return
		// }

		// build the path for the stat request. User is needed for creating a path to the resource
		// p := h.r.FormValue("path")
		// p = path.Join("/", u.Username, p)

		// prepare stat call to get resource information
		statReq := storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: "/home/shared.txt",
				},
			},
		}

		statRes, err := c.Stat(ctx, &statReq)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
			WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
			return
		}

		// TODO(refs) set expiration date to whatever phoenix sends
		req := publicshareproviderv0alphapb.CreatePublicShareRequest{
			ResourceInfo: statRes.GetInfo(),
			Grant: &publicshareproviderv0alphapb.Grant{
				Expiration: &typespb.Timestamp{
					Nanos:   uint32(time.Now().Add(time.Duration(31536000)).Nanosecond()),
					Seconds: uint64(time.Now().Add(time.Duration(31536000)).Second()),
				}, // transform string date from request into timestamp
			},
		}

		createRes, err := c.CreatePublicShare(ctx, &req)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msgf("error creating a public share to resource id: %v", statRes.Info.GetId())
			WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "error creating public share", fmt.Errorf("error creating a public share to resource id: %v", statRes.Info.GetId()))
			return
		}

		if createRes.Status.Code != rpcpb.Code_CODE_OK {
			log.Debug().Err(errors.New("create public share failed")).Str("shares", "createShare").Msgf("create public share failed with status code: %v", createRes.Status.Code.String())
			WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "grpc create public share request failed", err)
			return
		}

		// build ocs response for Phoenix
		s := conversions.PublicShare2ShareData(createRes.Share)
		WriteOCSSuccess(h.w, h.r, s)

		return
	}

	WriteOCSError(w, r, MetaBadRequest.StatusCode, "unknown share type", nil)
}

// TODO sort out mapping, this is just a first guess
// TODO use roles to make this configurable
func asCS3Permissions(p int, rp *storageproviderv0alphapb.ResourcePermissions) *storageproviderv0alphapb.ResourcePermissions {
	if rp == nil {
		rp = &storageproviderv0alphapb.ResourcePermissions{}
	}

	if p&int(conversions.PermissionRead) != 0 {
		rp.ListContainer = true
		rp.ListGrants = true
		rp.ListFileVersions = true
		rp.ListRecycle = true
		rp.Stat = true
		rp.GetPath = true
		rp.GetQuota = true
		rp.InitiateFileDownload = true
	}
	if p&int(conversions.PermissionWrite) != 0 {
		rp.InitiateFileUpload = true
		rp.RestoreFileVersion = true
		rp.RestoreRecycleItem = true
	}
	if p&int(conversions.PermissionCreate) != 0 {
		rp.CreateContainer = true
		// FIXME permissions mismatch: double check create vs write file
		rp.InitiateFileUpload = true
		if p&int(conversions.PermissionWrite) != 0 {
			rp.Move = true // TODO move only when create and write?
		}
	}
	if p&int(conversions.PermissionDelete) != 0 {
		rp.Delete = true
		rp.PurgeRecycle = true
	}
	if p&int(conversions.PermissionShare) != 0 {
		rp.AddGrant = true
		rp.RemoveGrant = true // TODO when are you able to unshare / delete
		rp.UpdateGrant = true
	}
	return rp
}

func (h *SharesHandler) permissions2Role(p int) string {
	role := conversions.RoleLegacy
	if p == int(conversions.PermissionRead) {
		role = conversions.RoleViewer
	}
	if p&int(conversions.PermissionWrite) == 1 {
		role = conversions.RoleEditor
	}
	if p&int(conversions.PermissionShare) == 1 {
		role = conversions.RoleCoowner
	}
	return role
}

func (h *SharesHandler) role2CS3Permissions(r string) (*storageproviderv0alphapb.ResourcePermissions, error) {
	switch r {
	case conversions.RoleViewer:
		return &storageproviderv0alphapb.ResourcePermissions{
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
		return &storageproviderv0alphapb.ResourcePermissions{
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
		return &storageproviderv0alphapb.ResourcePermissions{
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

func (h *SharesHandler) updateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pval := r.FormValue("permissions")
	if pval == "" {
		WriteOCSError(w, r, MetaBadRequest.StatusCode, "permissions missing", nil)
		return
	}

	perm, err := strconv.Atoi(pval)
	if err != nil {
		WriteOCSError(w, r, MetaBadRequest.StatusCode, "permissions must be an integer", nil)
		return
	}

	shareID := strings.TrimLeft(r.URL.Path, "/")
	// TODO we need to lookup the storage that is responsible for this share

	uClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc client", err)
		return
	}

	uReq := &usershareproviderv0alphapb.UpdateShareRequest{
		Ref: &usershareproviderv0alphapb.ShareReference{
			Spec: &usershareproviderv0alphapb.ShareReference_Id{
				Id: &usershareproviderv0alphapb.ShareId{
					OpaqueId: shareID,
				},
			},
		},
		Field: &usershareproviderv0alphapb.UpdateShareRequest_UpdateField{
			Field: &usershareproviderv0alphapb.UpdateShareRequest_UpdateField_Permissions{
				Permissions: &usershareproviderv0alphapb.SharePermissions{
					// this completely overwrites the permissions for this user
					Permissions: asCS3Permissions(perm, nil),
				},
			},
		},
	}
	uRes, err := uClient.UpdateShare(ctx, uReq)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc update share request", err)
		return
	}

	if uRes.Status.Code != rpcpb.Code_CODE_OK {
		if uRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
			return
		}
		WriteOCSError(w, r, MetaServerError.StatusCode, "grpc update share request failed", err)
		return
	}

	gReq := &usershareproviderv0alphapb.GetShareRequest{
		Ref: &usershareproviderv0alphapb.ShareReference{
			Spec: &usershareproviderv0alphapb.ShareReference_Id{
				Id: &usershareproviderv0alphapb.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	}
	gRes, err := uClient.GetShare(ctx, gReq)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc get share request", err)
		return
	}

	if gRes.Status.Code != rpcpb.Code_CODE_OK {
		if gRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
			return
		}
		WriteOCSError(w, r, MetaServerError.StatusCode, "grpc get share request failed", err)
		return
	}

	share, err := h.userShare2ShareData(ctx, gRes.Share)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	WriteOCSSuccess(w, r, share)
}

func (h *SharesHandler) listShares() {
	shares := make([]*conversions.ShareData, 0)
	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}
	var err error

	p := h.r.URL.Query().Get("path")
	if p != "" {
		filters, err = h.addFilters()
		if err != nil {
			WriteOCSError(h.w, h.r, MetaServerError.StatusCode, err.Error(), err)
		}
	}

	userShares, err := h.listUserShares(filters)
	if err != nil {
		WriteOCSError(h.w, h.r, MetaServerError.StatusCode, err.Error(), err)
	}

	publicShares, err := h.listPublicShares()
	if err != nil {
		WriteOCSError(h.w, h.r, MetaServerError.StatusCode, err.Error(), err)
	}

	shares = append(shares, append(userShares, publicShares...)...)

	if h.isReshareRequest() {
		WriteOCSSuccess(h.w, h.r, &conversions.Element{Data: shares})
		return
	}

	WriteOCSSuccess(h.w, h.r, shares)
}

func (h *SharesHandler) isReshareRequest() bool {
	return h.r.URL.Query().Get("reshares") != ""
}

func (h *SharesHandler) listPublicShares() ([]*conversions.ShareData, error) {
	ctx := h.r.Context()
	log := appctx.GetLogger(ctx)

	// TODO(refs) why is this guard needed? Are we moving towards a gateway only for service discovery? without a gateway this is dead code.
	if h.gatewayAddr != "" {
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return nil, err
		}

		filters := []*publicshareproviderv0alphapb.ListPublicSharesRequest_Filter{}
		req := publicshareproviderv0alphapb.ListPublicSharesRequest{
			Filters: filters,
		}

		res, err := c.ListPublicShares(ctx, &req)
		if err != nil {
			return nil, err
		}

		ocsDataPayload := make([]*conversions.ShareData, 0)
		for _, share := range res.GetShare() {
			sClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
			if err != nil {
				return nil, err
			}

			statRequest := &storageproviderv0alphapb.StatRequest{
				Ref: &storageproviderv0alphapb.Reference{
					Spec: &storageproviderv0alphapb.Reference_Id{
						Id: share.ResourceId,
					},
				},
			}

			statResponse, err := sClient.Stat(ctx, statRequest)
			if err != nil {
				return nil, err
			}

			sData := conversions.PublicShare2ShareData(share)
			if statResponse.Status.Code != rpcpb.Code_CODE_OK {
				return nil, err
			}

			if h.addFileInfo(ctx, sData, statResponse.Info) != nil {
				return nil, err
			}

			log.Debug().Interface("share", share).Interface("info", statResponse.Info).Interface("shareData", share).Msg("mapped")
			ocsDataPayload = append(ocsDataPayload, sData)

		}

		return ocsDataPayload, nil
	}

	return nil, errors.New("bad request")
}

func (h *SharesHandler) addFilters() ([]*usershareproviderv0alphapb.ListSharesRequest_Filter, error) {
	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}
	var info *storageproviderv0alphapb.ResourceInfo
	ctx := h.r.Context()

	// // TODO guard against this
	// p := h.r.URL.Query().Get("path")

	// // we need to prefix the path with the user id
	// u, ok := user.ContextGetUser(ctx)
	// if !ok {
	// 	WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
	// 	return nil, errors.New("fixme")
	// }

	// fn := path.Join("/", u.Username, p)

	// first check if the file exists
	gwClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "error getting grpc storage provider client", err)
		return nil, err
	}

	statReq := &storageproviderv0alphapb.StatRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{
				// Path: "/home" + p,
				Path: "/home/shared.txt", // TODO(refs) remove this hardcoded url. /home/file.ext works whereas /home/username/file.ext crashes.
			},
		},
	}

	res, err := gwClient.Stat(ctx, statReq)
	if err != nil {
		WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "error sending a grpc stat request", err)
		return nil, err
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			WriteOCSError(h.w, h.r, MetaNotFound.StatusCode, "not found", nil)
			return filters, errors.New("fixme")
		}
		WriteOCSError(h.w, h.r, MetaServerError.StatusCode, "grpc stat request penis failed", err)
		return filters, errors.New("fixme")
	}

	info = res.Info

	filters = append(filters, &usershareproviderv0alphapb.ListSharesRequest_Filter{
		Type: usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID,
		Term: &usershareproviderv0alphapb.ListSharesRequest_Filter_ResourceId{
			ResourceId: info.Id,
		},
	})

	return filters, nil
}

func (h *SharesHandler) listUserShares(filters []*usershareproviderv0alphapb.ListSharesRequest_Filter) ([]*conversions.ShareData, error) {
	var rInfo *storageproviderv0alphapb.ResourceInfo
	ctx := h.r.Context()
	log := appctx.GetLogger(ctx)

	lsUserSharesRequest := usershareproviderv0alphapb.ListSharesRequest{
		Filters: filters,
	}

	ocsDataPayload := make([]*conversions.ShareData, 0)
	if h.gatewayAddr != "" {
		// get a connection to the users share provider
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return nil, err
		}

		// do list shares request. unfiltered
		lsUserSharesResponse, err := c.ListShares(ctx, &lsUserSharesRequest)
		if err != nil {
			return nil, err
		}

		if lsUserSharesResponse.Status.Code != rpcpb.Code_CODE_OK {
			return nil, err
		}

		// build OCS response payload
		for _, s := range lsUserSharesResponse.Shares {
			share, err := h.userShare2ShareData(ctx, s)
			if err != nil {
				return nil, err
			}

			// check if the resource exists
			sClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
			if err != nil {
				return nil, err
			}

			// prepare the stat request
			statReq := &storageproviderv0alphapb.StatRequest{
				// prepare the reference
				Ref: &storageproviderv0alphapb.Reference{
					// using ResourceId from the share
					Spec: &storageproviderv0alphapb.Reference_Id{Id: s.ResourceId},
				},
			}

			statResponse, err := sClient.Stat(ctx, statReq)
			if err != nil {
				return nil, err
			}

			if statResponse.Status.Code != rpcpb.Code_CODE_OK {
				return nil, err
			}

			if h.addFileInfo(ctx, share, statResponse.Info) != nil {
				return nil, err
			}

			log.Debug().Interface("share", s).Interface("info", rInfo).Interface("shareData", share).Msg("mapped")
			ocsDataPayload = append(ocsDataPayload, share)
		}
	}

	return ocsDataPayload, nil
}

func (h *SharesHandler) addFileInfo(ctx context.Context, s *conversions.ShareData, info *storageproviderv0alphapb.ResourceInfo) error {
	if info != nil {
		// TODO The owner is not set in the storage stat metadata ...
		s.MimeType = info.MimeType
		// TODO STime:     &typespb.Timestamp{Seconds: info.Mtime.Seconds, Nanos: info.Mtime.Nanos},
		s.StorageID = info.Id.StorageId
		// TODO Storage: int
		s.ItemSource = info.Id.OpaqueId
		s.FileSource = info.Id.OpaqueId
		s.FileTarget = path.Join("/", path.Base(info.Path))
		s.Path = info.Path // TODO hm this might have to be relative to the users home ...
		// TODO FileParent:
		// item type
		s.ItemType = conversions.ResourceType(info.GetType()).String()

		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return err
		}

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			s.UIDFileOwner = UserIDToString(info.Owner)
		}
		if s.DisplaynameFileOwner == "" && info.Owner != nil {
			owner, err := c.GetUser(ctx, &userproviderv0alphapb.GetUserRequest{
				UserId: info.Owner,
			})
			if err != nil {
				return err
			}
			s.DisplaynameFileOwner = owner.GetUser().DisplayName
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			s.UIDOwner = UserIDToString(info.Owner)
		}
		if s.DisplaynameOwner == "" && info.Owner != nil {
			owner, err := c.GetUser(ctx, &userproviderv0alphapb.GetUserRequest{
				UserId: info.Owner,
			})
			if err != nil {
				return err
			}
			s.DisplaynameOwner = owner.GetUser().DisplayName
		}
	}
	return nil
}

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func (h *SharesHandler) userShare2ShareData(ctx context.Context, share *usershareproviderv0alphapb.Share) (*conversions.ShareData, error) {
	sd := &conversions.ShareData{
		Permissions: conversions.UserSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   conversions.ShareTypeUser,
	}

	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		return nil, err
	}

	if share.Creator != nil {
		if creator, err := c.GetUser(ctx, &userproviderv0alphapb.GetUserRequest{
			UserId: share.Creator,
		}); err == nil {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDOwner = UserIDToString(share.Creator)
			sd.DisplaynameOwner = creator.GetUser().DisplayName
		} else {
			return nil, err
		}
	}
	if share.Owner != nil {
		if owner, err := c.GetUser(ctx, &userproviderv0alphapb.GetUserRequest{
			UserId: share.Owner,
		}); err == nil {
			sd.UIDFileOwner = UserIDToString(share.Owner)
			sd.DisplaynameFileOwner = owner.GetUser().DisplayName
		} else {
			return nil, err
		}
	}
	if share.Grantee.Id != nil {
		if grantee, err := c.GetUser(ctx, &userproviderv0alphapb.GetUserRequest{
			UserId: share.Grantee.GetId(),
		}); err == nil {
			sd.ShareWith = UserIDToString(share.Grantee.Id)
			sd.ShareWithDisplayname = grantee.GetUser().DisplayName
		} else {
			return nil, err
		}
	}
	if share.Id != nil && share.Id.OpaqueId != "" {
		sd.ID = share.Id.OpaqueId
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd, nil
}

// SharesData holds a list of share data
type SharesData struct {
	Shares []*conversions.ShareData `json:"element" xml:"element"`
}
