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
	"errors"
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
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/ocssvc/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/user"
)

// SharesHandler implements the ownCloud sharing API
type SharesHandler struct {
	gatewaySvc          string
	userManager         user.Manager
	publicSharesManager publicshare.Manager
}

func (h *SharesHandler) init(c *Config) error {

	// TODO(jfd) lookup correct storage, for now this always uses the configured storage driver, maybe the combined storage can delegate this?
	h.gatewaySvc = c.GatewaySvc

	userManager, err := conversions.GetUserManager(c.UserManager, c.UserManagers)
	if err != nil {
		return err
	}
	h.userManager = userManager

	publicShareManager, err := conversions.GetPublicShareManager(c.PublicShareManager, c.PublicShareManagers)
	if err != nil {
		return err
	}

	h.userManager = userManager
	h.publicSharesManager = publicShareManager

	return nil
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		switch r.Method {
		case "OPTIONS":
			w.WriteHeader(http.StatusOK) // TODO cors?
			return
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
				role = conversions.RoleViewer
			} else {
				pint, err = strconv.Atoi(pval)
				if err != nil {
					WriteOCSError(w, r, MetaBadRequest.StatusCode, "permissions must be an integer", nil)
					return
				}
				role = conversions.Permissions2Role(pint)
			}
		}

		// map role to permissions
		var permissions *storageproviderv0alphapb.ResourcePermissions
		permissions, err = conversions.Role2CS3Permissions(role)
		if err != nil {
			log.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
			permissions = conversions.AsCS3Permissions(pint, nil)
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

	if shareType == int(conversions.ShareTypePublicLink) {
		// create a public link share
		// get a connection to the public shares service
		c, err := pool.GetPublicShareProviderClient(h.gatewaySvc)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msg("error creating public link share")
			WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("error getting a connection to a public shares provider"))
			return
		}

		u, ok := user.ContextGetUser(ctx)
		if !ok {
			WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
			return
		}

		// build the path for the stat request. User is needed for creating a path to the resource
		p := r.FormValue("path")
		p = path.Join("/", u.Username, p)

		// prepare stat call to get resource information
		statReq := storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: p,
				},
			},
		}

		// for launching a stat call we need a connection to the storage provicer
		spConn, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msg("error connecting to storage provider")
			WriteOCSError(w, r, MetaServerError.StatusCode, "shares", fmt.Errorf("error getting a connection to a storage provider"))
			return
		}

		statRes, err := spConn.Stat(ctx, &statReq)
		if err != nil {
			log.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
			WriteOCSError(w, r, MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
			return
		}

		// TODO(refs) phoenix is not setting expiration. Default to (now + 1 year?)
		// create public share request.
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
			WriteOCSError(w, r, MetaServerError.StatusCode, "error creating public share", fmt.Errorf("error creating a public share to resource id: %v", statRes.Info.GetId()))
			return
		}

		if createRes.Status.Code != rpcpb.Code_CODE_OK {
			log.Debug().Err(errors.New("create public share failed")).Str("shares", "createShare").Msgf("create public share failed with status code: %v", createRes.Status.Code.String())
			WriteOCSError(w, r, MetaServerError.StatusCode, "grpc create public share request failed", err)
			return
		}

		// build ocs response for Phoenix
		s := conversions.PublicShare2ShareData(createRes.Share)
		WriteOCSSuccess(w, r, s)

		return
	}

	WriteOCSError(w, r, MetaBadRequest.StatusCode, "unknown share type", nil)
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
					Permissions: conversions.AsCS3Permissions(perm, nil),
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

// this probablt has to be handled at the handler level. v1 != v2. check ocssvc.go:87
func isReshare(req *http.Request) bool {
	return req.URL.Query().Get("reshares") != ""
}

func (h *SharesHandler) addFilters(w http.ResponseWriter, r *http.Request) ([]*usershareproviderv0alphapb.ListSharesRequest_Filter, error) {
	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}
	var info *storageproviderv0alphapb.ResourceInfo
	ctx := r.Context()

	// TODO guard against this
	p := r.URL.Query().Get("path")

	// we need to prefix the path with the user id
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return nil, errors.New("fixme")
	}

	fn := path.Join("/", u.Username, p)

	// first check if the file exists
	sClient, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error getting grpc storage provider client", err)
		return nil, err
	}

	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
	}
	req := &storageproviderv0alphapb.StatRequest{Ref: ref}
	res, err := sClient.Stat(ctx, req)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error sending a grpc stat request", err)
		return nil, err
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			WriteOCSError(w, r, MetaNotFound.StatusCode, "not found", nil)
			return filters, errors.New("fixme")
		}
		WriteOCSError(w, r, MetaServerError.StatusCode, "grpc stat request failed", err)
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

func (h *SharesHandler) listShares(w http.ResponseWriter, r *http.Request) {
	shares := make([]*conversions.ShareData, 0)
	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}
	var err error

	// listing single file collaborators (path is present)
	p := r.URL.Query().Get("path")
	if p != "" {
		filters, err = h.addFilters(w, r)
		if err != nil {
			WriteOCSError(w, r, MetaServerError.StatusCode, err.Error(), err)
		}
	}

	userShares, err := h.listUserShares(w, r, filters)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, err.Error(), err)
	}

	publicShares, err := h.listPublicShares(w, r)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, err.Error(), err)
	}

	shares = append(shares, append(userShares, publicShares...)...)

	if isReshare(r) {
		WriteOCSSuccess(w, r, &conversions.Element{Data: shares})
		return
	}

	WriteOCSSuccess(w, r, shares)
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

	matches := make([]*conversions.MatchData, 0, len(users))

	for _, user := range users {
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

func (h *SharesHandler) listPublicShares(w http.ResponseWriter, r *http.Request) ([]*conversions.ShareData, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// TODO(refs) why is this guard needed?
	// TODO(refs) get rid of the pointer receiver since it hides more information than benefits provides
	if h.gatewaySvc != "" {
		// get a connection to the public shares provider
		publicSharesProvider, err := pool.GetPublicShareProviderClient(h.gatewaySvc)
		if err != nil {
			return nil, err
		}

		// prepare a listPublicShares request
		filters := []*publicshareproviderv0alphapb.ListPublicSharesRequest_Filter{}
		req := publicshareproviderv0alphapb.ListPublicSharesRequest{
			Filters: filters,
		}

		list, err := publicSharesProvider.ListPublicShares(ctx, &req)
		if err != nil {
			return nil, err
		}

		// In order to return a OCS payload we need to do a series of transformations:
		ocsDataPayload := make([]*conversions.ShareData, 0)
		for _, share := range list.Share {
			sData := conversions.PublicShare2ShareData(share)
			sClient, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
			if err != nil {
				return nil, err
			}

			// prepare the stat request
			statRequest := &storageproviderv0alphapb.StatRequest{
				// prepare the reference
				Ref: &storageproviderv0alphapb.Reference{
					// using ResourceId from the share
					Spec: &storageproviderv0alphapb.Reference_Id{
						Id: share.ResourceId,
					},
				},
			}

			statResponse, err := sClient.Stat(ctx, statRequest)
			if err != nil {
				return nil, err
			}

			if statResponse.Status.Code != rpcpb.Code_CODE_OK {
				return nil, err
			}

			// add file info to share data
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

func (h *SharesHandler) listUserShares(w http.ResponseWriter, r *http.Request, filters []*usershareproviderv0alphapb.ListSharesRequest_Filter) ([]*conversions.ShareData, error) {
	var rInfo *storageproviderv0alphapb.ResourceInfo
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	lsUserSharesRequest := usershareproviderv0alphapb.ListSharesRequest{
		Filters: filters,
	}

	ocsDataPayload := make([]*conversions.ShareData, 0)
	if h.gatewaySvc != "" {
		// get a connection to the users share provider
		userShareProviderClient, err := pool.GetUserShareProviderClient(h.gatewaySvc)
		if err != nil {
			return nil, err
		}

		// do list shares request. unfiltered
		lsUserSharesResponse, err := userShareProviderClient.ListShares(ctx, &lsUserSharesRequest)
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
			sClient, err := pool.GetStorageProviderServiceClient(h.gatewaySvc)
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

// glue code between cs3apis / ocs
// TODO(refs) get rid of pointer receiver
func (h *SharesHandler) addFileInfo(ctx context.Context, s *conversions.ShareData, info *storageproviderv0alphapb.ResourceInfo) error {
	if info != nil {
		// TODO The owner is not set in the storage stat metadata ...
		// TODO FileParent:
		// TODO STime:     &typespb.Timestamp{Seconds: info.Mtime.Seconds, Nanos: info.Mtime.Nanos},
		// TODO Storage: int
		s.MimeType = info.MimeType
		s.StorageID = info.Id.StorageId
		s.ItemSource = info.Id.OpaqueId
		s.FileSource = info.Id.OpaqueId
		s.FileTarget = path.Join("/", path.Base(info.Path))
		s.Path = info.Path // TODO hm this might have to be relative to the users home ...
		// item type
		s.ItemType = conversions.ResourceType(info.GetType()).String()

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			s.UIDFileOwner = UserIDToString(info.Owner)
		}

		// user shares
		if s.DisplaynameFileOwner == "" && info.Owner != nil && s.ShareType == 0 {
			owner, err := h.userManager.GetUser(ctx, info.Owner)
			if err != nil {
				return err
			}
			s.DisplaynameFileOwner = owner.DisplayName
		} else {
			// TODO(refs) fill with contextual user info
			s.DisplaynameFileOwner = "fixme"
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			s.UIDOwner = UserIDToString(info.Owner)
		}

		// user shares
		if s.DisplaynameOwner == "" && info.Owner != nil && s.ShareType == 0 {
			owner, err := h.userManager.GetUser(ctx, info.Owner)
			if err != nil {
				return err
			}
			s.DisplaynameOwner = owner.DisplayName
		} else {
			// TODO(refs) fill with contextual user info
			s.DisplaynameOwner = "fixme"
		}
	}
	return nil
}

// ===========
// Conversions
// ===========

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func (h *SharesHandler) userShare2ShareData(ctx context.Context, share *usershareproviderv0alphapb.Share) (*conversions.ShareData, error) {
	sd := &conversions.ShareData{
		Permissions: conversions.UserSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   conversions.ShareTypeUser,
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

func (h *SharesHandler) userAsMatch(u *authv0alphapb.User) *conversions.MatchData {
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
