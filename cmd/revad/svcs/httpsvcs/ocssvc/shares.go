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

package ocssvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
	usermgr "github.com/cs3org/reva/pkg/user/manager/registry"
)

// SharesHandler implements the ownCloud sharing API
type SharesHandler struct {
	gatewaySvc  string
	userManager user.Manager
}

func (h *SharesHandler) init(c *Config) error {

	// TODO(jfd) lookup correct storage, for now this always uses the configured storage driver, maybe the combined storage can delegate this?
	h.gatewaySvc = c.GatewaySvc

	userManager, err := getUserManager(c.UserManager, c.UserManagers)
	if err != nil {
		return err
	}
	h.userManager = userManager
	return nil
}

func getUserManager(manager string, m map[string]map[string]interface{}) (user.Manager, error) {
	if f, ok := usermgr.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for user manager", manager)
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		switch r.Method {
		case "GET":
			h.listShares(w, r)
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
	// TODO sanitize query

	users, err := h.userManager.FindUsers(ctx, search)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error searching users", err)
		return
	}

	log.Debug().Int("count", len(users)).Str("search", search).Msg("users found")

	matches := []*MatchData{}

	for _, user := range users {
		match := h.userAsMatch(user)
		log.Debug().Interface("user", user).Interface("match", match).Msg("mapped")
		matches = append(matches, match)
	}

	WriteOCSSuccess(w, r, &ShareeData{
		Exact: &ExactMatchesData{
			Users:   []*MatchData{},
			Groups:  []*MatchData{},
			Remotes: []*MatchData{},
		},
		Users:   matches,
		Groups:  []*MatchData{},
		Remotes: []*MatchData{},
	})
}

func (h *SharesHandler) userAsMatch(u *authv0alphapb.User) *MatchData {
	return &MatchData{
		Label: u.DisplayName,
		Value: &MatchValueData{
			ShareType: int(shareTypeUser),
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

	if shareType == int(shareTypeUser) {

		// if user sharing is disabled
		if h.gatewaySvc == "" {
			WriteOCSError(w, r, MetaServerError.StatusCode, "user sharing service not configured", nil)
			return
		}

		shareWith := r.FormValue("shareWith")
		if shareWith == "" {
			WriteOCSError(w, r, MetaBadRequest.StatusCode, "missing shareWith", nil)
			return
		}

		// find recipient based on username
		users, err := h.userManager.FindUsers(ctx, shareWith)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error searching recipient", err)
			return
		}
		var recipient *authv0alphapb.User
		for _, user := range users {
			if user.Username == shareWith {
				recipient = user
				break
			}
		}

		// we need to prefix the path with the user id
		u, ok := user.ContextGetUser(ctx)
		if !ok {
			WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
			return
		}

		// TODO how do we get the home of a user? The path in the sharing api is relative to the users home
		p := r.FormValue("path")
		p = path.Join("/", u.Username, p)

		var pint int

		role := r.FormValue("role")
		// 2. if we don't have a role try to map the permissions
		if role == "" {
			pval := r.FormValue("permissions")
			if pval == "" {
				// by default only allow read permissions / assign viewer role
				role = roleViewer
			} else {
				pint, err = strconv.Atoi(pval)
				if err != nil {
					WriteOCSError(w, r, MetaBadRequest.StatusCode, "permissions must be an integer", nil)
					return
				}
				role = h.permissions2Role(pint)
			}
		}

		// map role to permissions

		var permissions *storageproviderv0alphapb.ResourcePermissions
		permissions, err = h.role2CS3Permissions(role)
		if err != nil {
			log.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
			permissions = asCS3Permissions(pint, nil)
		}

		uClient, err := pool.GetUserShareProviderClient(h.gatewaySvc)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc client", err)
			return
		}
		roleMap := map[string]string{"name": role}
		val, err := json.Marshal(roleMap)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "could not encode role", err)
			return
		}

		statReq := &storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: p,
				},
			},
		}

		sClient, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
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

		req := &usershareproviderv0alphapb.CreateShareRequest{
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
		res, err := uClient.CreateShare(ctx, req)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc create share request", err)
			return
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
				return
			}
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc create share request failed", err)
			return
		}
		s, err := h.userShare2ShareData(ctx, res.Share)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error mapping share data", err)
			return
		}
		s.Path = r.FormValue("path") // use path without user prefix
		WriteOCSSuccess(w, r, s)
		return

	}

	WriteOCSError(w, r, MetaBadRequest.StatusCode, "unknown share type", nil)
}

// TODO sort out mapping, this is just a first guess
func permissions2OCSPermissions(p *storageproviderv0alphapb.ResourcePermissions) Permissions {
	permissions := permissionInvalid
	if p != nil {
		if p.ListContainer {
			permissions += permissionRead
		}
		if p.InitiateFileUpload {
			permissions += permissionWrite
		}
		if p.CreateContainer {
			permissions += permissionCreate
		}
		if p.Delete {
			permissions += permissionDelete
		}
		if p.AddGrant {
			permissions += permissionShare
		}
	}
	return permissions
}

// TODO sort out mapping, this is just a first guess
// TODO use roles to make this configurable
func asCS3Permissions(p int, rp *storageproviderv0alphapb.ResourcePermissions) *storageproviderv0alphapb.ResourcePermissions {
	if rp == nil {
		rp = &storageproviderv0alphapb.ResourcePermissions{}
	}

	if p&int(permissionRead) != 0 {
		rp.ListContainer = true
		rp.ListGrants = true
		rp.ListFileVersions = true
		rp.ListRecycle = true
		rp.Stat = true
		rp.GetPath = true
		rp.GetQuota = true
		rp.InitiateFileDownload = true
	}
	if p&int(permissionWrite) != 0 {
		rp.InitiateFileUpload = true
		rp.RestoreFileVersion = true
		rp.RestoreRecycleItem = true
	}
	if p&int(permissionCreate) != 0 {
		rp.CreateContainer = true
		// FIXME permissions mismatch: double check create vs write file
		rp.InitiateFileUpload = true
		if p&int(permissionWrite) != 0 {
			rp.Move = true // TODO move only when create and write?
		}
	}
	if p&int(permissionDelete) != 0 {
		rp.Delete = true
		rp.PurgeRecycle = true
	}
	if p&int(permissionShare) != 0 {
		rp.AddGrant = true
		rp.RemoveGrant = true // TODO when are you able to unshare / delete
		rp.UpdateGrant = true
	}
	return rp
}

func (h *SharesHandler) permissions2Role(p int) string {
	role := roleLegacy
	if p == int(permissionRead) {
		role = roleViewer
	}
	if p&int(permissionWrite) == 1 {
		role = roleEditor
	}
	if p&int(permissionShare) == 1 {
		role = roleCoowner
	}
	return role
}

func (h *SharesHandler) role2CS3Permissions(r string) (*storageproviderv0alphapb.ResourcePermissions, error) {
	switch r {
	case roleViewer:
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
	case roleEditor:
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
	case roleCoowner:
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

	uClient, err := pool.GetUserShareProviderClient(h.gatewaySvc)
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

func (h *SharesHandler) listShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}
	var info *storageproviderv0alphapb.ResourceInfo

	p := r.URL.Query().Get("path")
	log.Debug().Str("path", p).Msg("listShares")
	if p != "" {
		// we need to prefix the path with the user id
		u, ok := user.ContextGetUser(ctx)
		if !ok {
			WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
			return
		}
		// TODO how do we get the home of a user? The path in the sharing api is relative to the users home
		fn := path.Join("/", u.Username, p)

		log.Debug().Str("path", p).Str("fn", fn).Interface("user", u).Msg("resolved path for user")

		// first check if the file exists
		sClient, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc storage provider client", err)
			return
		}

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		}
		req := &storageproviderv0alphapb.StatRequest{Ref: ref}
		res, err := sClient.Stat(ctx, req)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc stat request", err)
			return
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
				return
			}
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc stat request failed", err)
			return
		}

		info = res.Info

		log.Debug().Interface("info", info).Msg("path found")

		filters = append(filters, &usershareproviderv0alphapb.ListSharesRequest_Filter{
			Type: usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID,
			Term: &usershareproviderv0alphapb.ListSharesRequest_Filter_ResourceId{
				ResourceId: info.Id,
			},
		})
	}

	shares := []*ShareData{}

	// fetch user shares if configured
	if h.gatewaySvc != "" {
		uClient, err := pool.GetUserShareProviderClient(h.gatewaySvc)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc user share handler client", err)
			return
		}
		req := &usershareproviderv0alphapb.ListSharesRequest{
			Filters: filters,
		}
		res, err := uClient.ListShares(ctx, req)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc list shares request", err)
			return
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
				return
			}
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc list shares request failed", err)
			return
		}
		for _, s := range res.Shares {
			share, err := h.userShare2ShareData(ctx, s)
			if err != nil {
				WriteOCSError(w, r, MetaServerError.StatusCode, "error mapping share data", err)
				return
			}
			if h.addFileInfo(ctx, share, info) != nil {
				WriteOCSError(w, r, MetaServerError.StatusCode, "error adding file info", err)
				return
			}
			log.Debug().Interface("share", s).Interface("info", info).Interface("shareData", share).Msg("mapped")
			shares = append(shares, share)
		}
	}
	// TODO fetch group shares
	// TODO fetch federated shares

	// fetch public link shares if configured
	/*
		if h.gatewaySvc != "" {
			pClient, err := pool.GetPublicShareProviderClient(h.gatewaySvc)
			if err != nil {
				WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc public share provider client", err)
				return
			}
			req := &publicshareproviderv0alphapb.ListPublicSharesRequest{}
			res, err := pClient.ListPublicShares(ctx, req)
			if err != nil {
				WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc list shares request", err)
				return
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
					WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
					return
				}
				WriteOCSError(w, r, MetaServerError.StatusCode, "grpc list shares request failed", err)
				return
			}
			for _, s := range res.Share {
				share := h.publicShare2ShareData(s)
				err := h.addFileInfo(ctx, share, info)
				if err != nil {
					WriteOCSError(w, r, MetaServerError.StatusCode, "error adding file info", err)
					return
				}
				log.Debug().Interface("share", s).Interface("info", info).Interface("shareData", share).Msg("mapped")
				shares = append(shares, share)
			}
		}
	*/

	WriteOCSSuccess(w, r, &SharesData{
		Shares: shares,
	})
}

func (h *SharesHandler) addFileInfo(ctx context.Context, s *ShareData, info *storageproviderv0alphapb.ResourceInfo) error {
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

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			s.UIDFileOwner = UserIDToString(info.Owner)
		}
		if s.DisplaynameFileOwner == "" && info.Owner != nil {
			owner, err := h.userManager.GetUser(ctx, info.Owner)
			if err != nil {
				return err
			}
			s.DisplaynameFileOwner = owner.DisplayName
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			s.UIDOwner = UserIDToString(info.Owner)
		}
		if s.DisplaynameOwner == "" && info.Owner != nil {
			owner, err := h.userManager.GetUser(ctx, info.Owner)
			if err != nil {
				return err
			}
			s.DisplaynameOwner = owner.DisplayName
		}
	}
	return nil
}

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func (h *SharesHandler) userShare2ShareData(ctx context.Context, share *usershareproviderv0alphapb.Share) (*ShareData, error) {
	sd := &ShareData{
		Permissions: userSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   shareTypeUser,
	}
	if share.Creator != nil {
		if creator, err := h.userManager.GetUser(ctx, share.Creator); err == nil {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDOwner = UserIDToString(share.Creator)
			sd.DisplaynameOwner = creator.DisplayName
		} else {
			return nil, err
		}
	}
	if share.Owner != nil {
		if owner, err := h.userManager.GetUser(ctx, share.Owner); err == nil {
			sd.UIDFileOwner = UserIDToString(share.Owner)
			sd.DisplaynameFileOwner = owner.DisplayName
		} else {
			return nil, err
		}
	}
	if share.Grantee.Id != nil {
		if grantee, err := h.userManager.GetUser(ctx, share.Grantee.Id); err == nil {
			sd.ShareWith = UserIDToString(share.Grantee.Id)
			sd.ShareWithDisplayname = grantee.DisplayName
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

func userSharePermissions2OCSPermissions(sp *usershareproviderv0alphapb.SharePermissions) Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return permissionInvalid
}

func publicSharePermissions2OCSPermissions(sp *publicshareproviderv0alphapb.PublicSharePermissions) Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return permissionInvalid
}

func (h *SharesHandler) publicShare2ShareData(share *publicshareproviderv0alphapb.PublicShare) *ShareData {
	sd := &ShareData{
		ID: share.Id.OpaqueId,
		// TODO map share.resourceId to path and storage ... requires a stat call
		// share.permissions ar mapped below
		Permissions: publicSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   shareTypePublicLink,
		UIDOwner:    UserIDToString(share.Creator),
		// TODO lookup user metadata
		//DisplaynameOwner:     creator.DisplayName,
		STime:        share.Ctime.Seconds, // TODO CS3 api birth time = btime
		UIDFileOwner: UserIDToString(share.Owner),
		// TODO lookup user metadata
		//DisplaynameFileOwner: owner.DisplayName,
		Token:      share.Token,
		Expiration: timestampToExpiration(share.Expiration),
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd
}

// timestamp is assumed to be UTC ... just human readable ...
// FIXME and ambiguous / error prone because there is no time zone ...
func timestampToExpiration(t *typespb.Timestamp) string {
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).Format("2006-01-02 15:05:05")
}

// SharesData holds a list of share data
type SharesData struct {
	Shares []*ShareData `json:"element" xml:"element"`
}

// ShareType indicates the type of share
// TODO Phoenix should be able to handle int shareType in json
type ShareType int

const (
	shareTypeUser ShareType = 0
	//	shareTypeGroup               ShareType = 1
	shareTypePublicLink ShareType = 3
	//	shareTypeFederatedCloudShare ShareType = 6
)

// Permissions reflects the CRUD permissions used in the OCS sharing API
type Permissions uint

const (
	permissionInvalid Permissions = 0
	permissionRead    Permissions = 1
	permissionWrite   Permissions = 2
	permissionCreate  Permissions = 4
	permissionDelete  Permissions = 8
	permissionShare   Permissions = 16
	//permissionAll     Permissions = 31
)

const (
	roleLegacy  string = "legacy"
	roleViewer  string = "viewer"
	roleEditor  string = "editor"
	roleCoowner string = "coowner"
)

// ShareData holds share data, see https://doc.owncloud.com/server/developer_manual/core/ocs-share-api.html#response-attributes-1
type ShareData struct {
	// TODO int?
	ID string `json:"id" xml:"id"`
	// The share’s type. This can be one of:
	// 0 = user
	// 1 = group
	// 3 = public link
	// 6 = federated cloud share
	ShareType ShareType `json:"share_type" xml:"share_type"`
	// The username of the owner of the share.
	UIDOwner string `json:"uid_owner" xml:"uid_owner"`
	// The display name of the owner of the share.
	DisplaynameOwner string `json:"displayname_owner" xml:"displayname_owner"`
	// The permission attribute set on the file. Options are:
	// * 1 = Read
	// * 2 = Update
	// * 4 = Create
	// * 8 = Delete
	// * 16 = Share
	// * 31 = All permissions
	// The default is 31, and for public shares is 1.
	// TODO we should change the default to read only
	Permissions Permissions `json:"permissions" xml:"permissions"`
	// The UNIX timestamp when the share was created.
	STime uint64 `json:"stime" xml:"stime"`
	// ?
	Parent string `json:"parent" xml:"parent"`
	// The UNIX timestamp when the share expires.
	Expiration string `json:"expiration" xml:"expiration"`
	// The public link to the item being shared.
	Token string `json:"token" xml:"token"`
	// The unique id of the user that owns the file or folder being shared.
	UIDFileOwner string `json:"uid_file_owner" xml:"uid_file_owner"`
	// The display name of the user that owns the file or folder being shared.
	DisplaynameFileOwner string `json:"displayname_file_owner" xml:"displayname_file_owner"`
	// The path to the shared file or folder.
	Path string `json:"path" xml:"path"`
	// The type of the object being shared. This can be one of file or folder.
	ItemType string `json:"item_type" xml:"item_type"`
	// The RFC2045-compliant mimetype of the file.
	MimeType  string `json:"mimetype" xml:"mimetype"`
	StorageID string `json:"storage_id" xml:"storage_id"`
	Storage   uint64 `json:"storage" xml:"storage"`
	// The unique node id of the item being shared.
	// TODO int?
	ItemSource string `json:"item_source" xml:"item_source"`
	// The unique node id of the item being shared. For legacy reasons item_source and file_source attributes have the same value.
	// TODO int?
	FileSource string `json:"file_source" xml:"file_source"`
	// The unique node id of the parent node of the item being shared.
	// TODO int?
	FileParent string `json:"file_parent" xml:"file_parent"`
	// The name of the shared file.
	FileTarget string `json:"file_target" xml:"file_target"`
	// The uid of the receiver of the file. This is either
	// - a GID (group id) if it is being shared with a group or
	// - a UID (user id) if the share is shared with a user.
	ShareWith string `json:"share_with" xml:"share_with"`
	// The display name of the receiver of the file.
	ShareWithDisplayname string `json:"share_with_displayname" xml:"share_with_displayname"`
	// Whether the recipient was notified, by mail, about the share being shared with them.
	MailSend string `json:"mail_send" xml:"mail_send"`
	// A (human-readable) name for the share, which can be up to 64 characters in length
	Name string `json:"name" xml:"name"`
}

// ShareeData holds share recipaent search results
type ShareeData struct {
	Exact   *ExactMatchesData `json:"exact" xml:"exact"`
	Users   []*MatchData      `json:"users" xml:"users"`
	Groups  []*MatchData      `json:"groups" xml:"groups"`
	Remotes []*MatchData      `json:"remotes" xml:"remotes"`
}

// ExactMatchesData hold exact matches
type ExactMatchesData struct {
	Users   []*MatchData `json:"users" xml:"users"`
	Groups  []*MatchData `json:"groups" xml:"groups"`
	Remotes []*MatchData `json:"remotes" xml:"remotes"`
}

// MatchData describes a single match
type MatchData struct {
	Label string          `json:"label" xml:"label"`
	Value *MatchValueData `json:"value" xml:"value"`
}

// MatchValueData holds the type and actual value
type MatchValueData struct {
	ShareType int    `json:"shareType" xml:"shareType"`
	ShareWith string `json:"shareWith" xml:"shareWith"`
}
