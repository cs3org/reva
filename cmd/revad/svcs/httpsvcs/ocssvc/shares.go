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
	"time"

	publicsharev0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshare/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	shareregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/shareregistry/v0alpha"
	sharetypespb "github.com/cs3org/go-cs3apis/cs3/sharetypes"
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
	shareRegistrySvc string
	conn             *grpc.ClientConn
	client           shareregistryv0alphapb.ShareRegistryServiceClient
}

func (h *SharesHandler) init(c *Config) {
	h.shareRegistrySvc = ":9999" // TODO(jfd) fixme read from config
}

func (h *SharesHandler) getConn() (*grpc.ClientConn, error) {
	if h.conn != nil {
		return h.conn, nil
	}

	conn, err := grpc.Dial(h.shareRegistrySvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (h *SharesHandler) getClient() (shareregistryv0alphapb.ShareRegistryServiceClient, error) {
	if h.client != nil {
		return h.client, nil
	}

	conn, err := h.getConn()
	if err != nil {
		return nil, err
	}
	h.client = shareregistryv0alphapb.NewShareRegistryServiceClient(conn)
	return h.client, nil
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		// TODO PUT vs GET
		h.listShares(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (h *SharesHandler) listShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := h.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &shareregistryv0alphapb.ListShareProvidersRequest{}
	res, err := client.ListShareProviders(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc ListShareProviders request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpcpb.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

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

	for _, p := range res.Providers {
		// query this provider
		pConn, err := grpc.Dial(p.Address, grpc.WithInsecure())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch p.ShareType {
		case sharetypespb.ShareType_SHARE_TYPE_USER:
			pClient := usershareproviderv0alphapb.NewUserShareProviderServiceClient(pConn)
			req := &usershareproviderv0alphapb.ListSharesRequest{
				Filters: filters,
			}
			res, err := pClient.ListShares(ctx, req)
			if err != nil {
				log.Error().Err(err).Str("address", p.Address).Msg("error sending a grpc list shares request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			for _, s := range res.Share {
				shares = append(shares, h.userShare2ShareData(s))
			}
		case sharetypespb.ShareType_SHARE_TYPE_PUBLIC_LINK:
			pClient := publicsharev0alphapb.NewPublicShareProviderServiceClient(pConn)
			req := &publicsharev0alphapb.ListPublicSharesRequest{}
			res, err := pClient.ListPublicShares(ctx, req)
			if err != nil {
				log.Error().Err(err).Msg("error sending a grpc stat request")
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

	}
	// get shares registry
	// get share provider

	res2 := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: SharesData{
				Shares: shares,
			},
		},
	}

	err = WriteOCSResponse(w, r, res2)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// TODO sort out mapping, this is just a first guess
func userSharePermissions2OCSPermissions(sp *usershareproviderv0alphapb.SharePermissions) Permissions {
	permissions := PermissionInvalid
	if sp != nil {
		p := sp.GetPermissions()
		if p != nil {
			if p.Stat && p.ListContainer && p.InitiateFileDownload {
				permissions += PermissionRead
			}
			if p.InitiateFileUpload {
				permissions += PermissionWrite
			}
			if p.CreateContainer {
				permissions += PermissionCreate
			}
			if p.Delete {
				permissions += PermissionDelete
			}
			if p.AddGrant {
				permissions += PermissionShare
			}
		}
	}
	return permissions
}

func (h *SharesHandler) userShare2ShareData(share *usershareproviderv0alphapb.Share) *ShareData {
	sd := &ShareData{
		ID: share.Id.OpaqueId,
		// TODO map share.resourceId to path and storage ... requires a stat call
		// share.permissions ar mapped below
		Permissions:          userSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:            ShareTypeUser,
		UIDOwner:             share.Creator,             // TODO this should come from a user object, not a string
		DisplaynameOwner:     share.Creator,             // TODO this should come from a user object, not a string
		STime:                share.Ctime.Seconds,       // TODO CS3 api birth time = btime
		UIDFileOwner:         share.Owner,               // TODO this should come from a user object, not a string
		DisplaynameFileOwner: share.Owner,               // TODO this should come from a user object, not a string
		ShareWith:            share.Grantee.Id.OpaqueId, // TODO cs3 api should pass around the minimal user data: id (sub&iss), username, email, displayname and avatar link
		ShareWithDisplayname: share.Grantee.Id.OpaqueId, // TODO this should come from a user object, not a string
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd
}

// TODO sort out mapping, this is just a first guess
func publicSharePermissions2OCSPermissions(sp *publicsharev0alphapb.PublicSharePermissions) Permissions {
	permissions := PermissionInvalid
	if sp != nil {
		p := sp.GetPermissions()
		if p != nil {
			if p.Stat && p.ListContainer && p.InitiateFileDownload {
				permissions += PermissionRead
			}
			if p.InitiateFileUpload {
				permissions += PermissionWrite
			}
			if p.CreateContainer {
				permissions += PermissionCreate
			}
			if p.Delete {
				permissions += PermissionDelete
			}
			if p.AddGrant {
				permissions += PermissionShare
			}
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
		ShareType:            ShareTypePublicLink,
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

// timestamp is assumed to be UTC ... just human readible ...
// FIXME and ambiguous / error prone because there is no time zone ...
func timestampToExpiration(t *typespb.Timestamp) string {
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).Format("2006-01-02 15:05:05")
}

// SharesData holds a list of share data
type SharesData struct {
	Shares []*ShareData `json:"element" xml:"element"`
}

type ShareType int

const (
	ShareTypeUser                ShareType = 0
	ShareTypeGroup               ShareType = 1
	ShareTypePublicLink          ShareType = 3
	ShareTypeFederatedCloudShare ShareType = 6
)

type Permissions uint

const (
	PermissionInvalid Permissions = 0
	PermissionRead    Permissions = 1
	PermissionWrite   Permissions = 2
	PermissionCreate  Permissions = 4
	PermissionDelete  Permissions = 8
	PermissionShare   Permissions = 16
	PermissionAll     Permissions = 31
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
