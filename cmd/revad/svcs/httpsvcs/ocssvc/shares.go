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
	"net/http"
	"strconv"
	"time"

	publicsharev0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshare/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
	"google.golang.org/grpc"
)

// SharesHandler implements the ownCloud sharing API
type SharesHandler struct {
	userShareProviderSVC   string
	uConn                  *grpc.ClientConn
	uClient                usershareproviderv0alphapb.UserShareProviderServiceClient
	publicShareProviderSVC string
	pConn                  *grpc.ClientConn
	pClient                publicsharev0alphapb.PublicShareProviderServiceClient
}

func (h *SharesHandler) init(c *Config) {
	h.userShareProviderSVC = c.UserShareProviderSVC
	h.publicShareProviderSVC = c.PublicShareProviderSVC
}

func (h *SharesHandler) getUConn() (*grpc.ClientConn, error) {
	if h.uConn != nil {
		return h.uConn, nil
	}

	conn, err := grpc.Dial(h.userShareProviderSVC, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	h.uConn = conn
	return h.uConn, nil
}

func (h *SharesHandler) getUClient() (usershareproviderv0alphapb.UserShareProviderServiceClient, error) {
	if h.uClient != nil {
		return h.uClient, nil
	}

	conn, err := h.getUConn()
	if err != nil {
		return nil, err
	}
	h.uClient = usershareproviderv0alphapb.NewUserShareProviderServiceClient(conn)
	return h.uClient, nil
}
func (h *SharesHandler) getPConn() (*grpc.ClientConn, error) {
	if h.pConn != nil {
		return h.pConn, nil
	}

	conn, err := grpc.Dial(h.userShareProviderSVC, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	h.pConn = conn
	return h.pConn, nil
}

func (h *SharesHandler) getPClient() (publicsharev0alphapb.PublicShareProviderServiceClient, error) {
	if h.pClient != nil {
		return h.pClient, nil
	}

	conn, err := h.getPConn()
	if err != nil {
		return nil, err
	}
	h.pClient = publicsharev0alphapb.NewPublicShareProviderServiceClient(conn)
	return h.pClient, nil
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
		default:
			http.Error(w, "Only GET and POST are allowed", http.StatusMethodNotAllowed)
		}
	case "sharees":
		h.findSharees(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (h *SharesHandler) findSharees(w http.ResponseWriter, r *http.Request) {
	// TODO implement search

	res := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: &ShareeData{
				Exact: &ExactMatchesData{
					Users:   []*MatchData{},
					Groups:  []*MatchData{},
					Remotes: []*MatchData{},
				},
				Users: []*MatchData{
					&MatchData{
						Label: "Aaliya Abernathy",
						Value: &MatchValueData{
							ShareType: int(shareTypeUser),
							ShareWith: "aaliya_abernathy",
						},
					},
				},
				Groups:  []*MatchData{},
				Remotes: []*MatchData{},
			},
		},
	}

	err := WriteOCSResponse(w, r, res)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (h *SharesHandler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		http.Error(w, "shareType must be an integer", http.StatusBadRequest)
		return
	}
	shareWith := r.FormValue("shareWith")

	if shareWith == "" {
		http.Error(w, "missing shareWith", http.StatusBadRequest)
		return
	}

	p := r.FormValue("path")
	// we need to prefix the path with the user id
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		http.Error(w, "missing user in context", http.StatusInternalServerError)
		return
	}
	p = u.Username + "/" + p
	share := &ShareData{}

	// by defailt only allow read permissions
	permissions := &storageproviderv0alphapb.ResourcePermissions{
		ListContainer:        true,
		Stat:                 true,
		InitiateFileDownload: true,
	}

	if shareType == int(shareTypeUser) {

		// if user sharing is enabled
		if h.userShareProviderSVC != "" {
			uClient, err := h.getUClient()
			if err != nil {
				log.Error().Err(err).Msg("error getting grpc client")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			req := &usershareproviderv0alphapb.CreateShareRequest{
				// Opaque: optional
				ResourceId: &storageproviderv0alphapb.ResourceId{
					StorageId: "TODO",
					OpaqueId:  p,
				},
				Grant: &usershareproviderv0alphapb.ShareGrant{
					Grantee: &storageproviderv0alphapb.Grantee{
						Type: storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER,
						Id: &typespb.UserId{
							// Idp: TODO get from where?
							OpaqueId: shareWith,
						},
					},
					Permissions: &usershareproviderv0alphapb.SharePermissions{
						Permissions: permissions,
					},
				},
			}
			res, err := uClient.CreateShare(ctx, req)
			if err != nil {
				log.Error().Err(err).Msg("error sending a grpc list shares request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			share = h.userShare2ShareData(res.Share)
			share.Path = r.FormValue("path") // use path without user prefix
		} else {
			http.Error(w, "user sharing service not configured", http.StatusServiceUnavailable)
			return
		}

	} else {
		http.Error(w, "unknown share type", http.StatusBadRequest)
		return
	}

	res := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: share,
		},
	}

	err = WriteOCSResponse(w, r, res)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (h *SharesHandler) listShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}

	path := r.URL.Query().Get("path")
	if path != "" {
		filters = append(filters, &usershareproviderv0alphapb.ListSharesRequest_Filter{
			Type: usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID,
			Term: &usershareproviderv0alphapb.ListSharesRequest_Filter_ResourceId{
				ResourceId: &storageproviderv0alphapb.ResourceId{
					StorageId: "", // TODO(jfd) lookup correct storage, for now this always uses the configured storage driver, maybe the combined storage can delegate this?
					OpaqueId:  path,
				},
			},
		})
	}

	shares := []*ShareData{}

	// fetch user shares if configured
	if h.userShareProviderSVC != "" {
		uClient, err := h.getUClient()
		if err != nil {
			log.Error().Err(err).Msg("error getting grpc client")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		req := &usershareproviderv0alphapb.ListSharesRequest{
			Filters: filters,
		}
		res, err := uClient.ListShares(ctx, req)
		if err != nil {
			log.Error().Err(err).Msg("error sending a grpc list shares request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, s := range res.Shares {
			shares = append(shares, h.userShare2ShareData(s))
		}
	}
	// TODO fetch group shares
	// TODO fetch federated shares

	// fetch public link shares if configured
	if h.publicShareProviderSVC != "" {
		pClient, err := h.getPClient()
		if err != nil {
			log.Error().Err(err).Msg("error getting grpc client")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		req := &publicsharev0alphapb.ListPublicSharesRequest{}
		res, err := pClient.ListPublicShares(ctx, req)
		if err != nil {
			log.Error().Err(err).Msg("error sending a grpc list shares request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, s := range res.Share {
			shares = append(shares, h.publicShare2ShareData(s))
		}
	}

	res2 := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: SharesData{
				Shares: shares,
			},
		},
	}

	err := WriteOCSResponse(w, r, res2)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func (h *SharesHandler) userShare2ShareData(share *usershareproviderv0alphapb.Share) *ShareData {
	creator := h.resolveUserString(share.Creator)
	owner := h.resolveUserString(share.Owner)
	grantee := h.resolveUserID(share.Grantee.Id)
	sd := &ShareData{
		ID: share.Id.OpaqueId,
		// TODO map share.resourceId to path and storage ... requires a stat call
		// share.permissions ar mapped below
		Permissions:          userSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:            shareTypeUser,
		UIDOwner:             creator.ID.String(),
		DisplaynameOwner:     creator.DisplayName,
		STime:                share.Ctime.Seconds, // TODO CS3 api birth time = btime
		UIDFileOwner:         owner.ID.String(),
		DisplaynameFileOwner: owner.DisplayName,
		ShareWith:            grantee.ID.String(),
		ShareWithDisplayname: grantee.DisplayName,
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd
}

func userSharePermissions2OCSPermissions(sp *usershareproviderv0alphapb.SharePermissions) Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return permissionInvalid
}

func publicSharePermissions2OCSPermissions(sp *publicsharev0alphapb.PublicSharePermissions) Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return permissionInvalid
}

// TODO sort out mapping, this is just a first guess
func permissions2OCSPermissions(p *storageproviderv0alphapb.ResourcePermissions) Permissions {
	permissions := permissionInvalid
	if p != nil {
		if p.Stat && p.ListContainer && p.InitiateFileDownload {
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

// TODO do user lookup and cache users
func (h *SharesHandler) resolveUserID(userID *typespb.UserId) *user.User {
	return &user.User{
		ID: &user.ID{
			IDP:      userID.Idp,
			OpaqueID: userID.OpaqueId,
		},
		DisplayName: "unknown",
	}
}

// TODO do user lookup and cache users
func (h *SharesHandler) resolveUserString(userID string) *user.User {
	return &user.User{
		ID: &user.ID{
			OpaqueID: userID,
		},
		Username:    userID,
		DisplayName: userID,
	}
}

func (h *SharesHandler) publicShare2ShareData(share *publicsharev0alphapb.PublicShare) *ShareData {
	creator := h.resolveUserID(share.Creator)
	owner := h.resolveUserID(share.Owner)
	sd := &ShareData{
		ID: share.Id.OpaqueId,
		// TODO map share.resourceId to path and storage ... requires a stat call
		// share.permissions ar mapped below
		Permissions:          publicSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:            shareTypePublicLink,
		UIDOwner:             creator.ID.String(),
		DisplaynameOwner:     creator.DisplayName,
		STime:                share.Ctime.Seconds, // TODO CS3 api birth time = btime
		UIDFileOwner:         owner.ID.String(),
		DisplaynameFileOwner: owner.DisplayName,
		Token:                share.Token,
		Expiration:           timestampToExpiration(share.Expiration),
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
type ShareType int

const (
	shareTypeUser                ShareType = 0
	shareTypeGroup               ShareType = 1
	shareTypePublicLink          ShareType = 3
	shareTypeFederatedCloudShare ShareType = 6
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
	permissionAll     Permissions = 31
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

type ShareeData struct {
	Exact   *ExactMatchesData `json:"exact" xml:"exact"`
	Users   []*MatchData      `json:"users" xml:"users"`
	Groups  []*MatchData      `json:"groups" xml:"groups"`
	Remotes []*MatchData      `json:"remotes" xml:"remotes"`
}

type ExactMatchesData struct {
	Users   []*MatchData `json:"users" xml:"users"`
	Groups  []*MatchData `json:"groups" xml:"groups"`
	Remotes []*MatchData `json:"remotes" xml:"remotes"`
}
type MatchData struct {
	Label string          `json:"label" xml:"label"`
	Value *MatchValueData `json:"value" xml:"value"`
}
type MatchValueData struct {
	ShareType int    `json:"shareType" xml:"shareType"`
	ShareWith string `json:"shareWith" xml:"shareWith"`
}
