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

package sql

import (
	"database/sql"
	"strconv"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v2/pkg/ocm/share"
)

// ShareType is the type of the share.
type ShareType int

// ItemType is the type of the shares resource.
type ItemType int

// AccessMethod is method granted by the sharer to access
// the shared resource.
type AccessMethod int

// Protocol is the protocol the recipient of the share
// uses to access the shared resource.
type Protocol int

// ShareState is the state of the share.
type ShareState int

const (
	// ShareTypeUser is used for a share to an user.
	ShareTypeUser ShareType = iota
	// ShareTypeGroup is used for a share to a group.
	ShareTypeGroup
)

const (
	// ShareStatePending is the state for a pending share.
	ShareStatePending ShareState = iota
	// ShareStateAccepted is the share for an accepted share.
	ShareStateAccepted
	// ShareStateRejected is the share for a rejected share.
	ShareStateRejected
)

const (
	// WebDAVAccessMethod indicates an access using WebDAV to the share.
	WebDAVAccessMethod AccessMethod = iota
	// WebappAccessMethod indicates an access using a collaborative
	// application to the share.
	WebappAccessMethod
	// TransferAccessMethod indicates a share for a transfer.
	TransferAccessMethod
)

const (
	// WebDAVProtocol is the WebDav protocol.
	WebDAVProtocol Protocol = iota
	// WebappProtocol is the Webapp protocol.
	WebappProtocol
	// TransferProtocol is the Transfer protocol.
	TransferProtocol
)

const (
	// ItemTypeFile is used when the shared resource is a file.
	ItemTypeFile ItemType = iota
	// ItemTypeFolder is used when the shared resource is a folder.
	ItemTypeFolder
)

func convertFromCS3OCMShareType(shareType ocm.ShareType) ShareType {
	switch shareType {
	case ocm.ShareType_SHARE_TYPE_USER:
		return ShareTypeUser
	case ocm.ShareType_SHARE_TYPE_GROUP:
		return ShareTypeGroup
	}
	return -1
}

func convertFromCS3OCMShareState(shareState ocm.ShareState) ShareState {
	switch shareState {
	case ocm.ShareState_SHARE_STATE_ACCEPTED:
		return ShareStateAccepted
	case ocm.ShareState_SHARE_STATE_PENDING:
		return ShareStatePending
	case ocm.ShareState_SHARE_STATE_REJECTED:
		return ShareStateRejected
	}
	return -1
}

func convertToCS3OCMShareState(state ShareState) ocm.ShareState {
	switch state {
	case ShareStateAccepted:
		return ocm.ShareState_SHARE_STATE_ACCEPTED
	case ShareStatePending:
		return ocm.ShareState_SHARE_STATE_PENDING
	case ShareStateRejected:
		return ocm.ShareState_SHARE_STATE_REJECTED
	}
	return ocm.ShareState_SHARE_STATE_INVALID
}

type dbShare struct {
	ID         int
	Token      string
	Prefix     string
	ItemSource string
	Name       string
	ShareWith  string
	Owner      string
	Initiator  string
	Ctime      int
	Mtime      int
	Expiration sql.NullInt64
	ShareType  ShareType
}

type dbAccessMethod struct {
	ShareID           string
	Type              AccessMethod
	WebDAVPermissions *int
	WebAppViewMode    *int
}

type dbReceivedShare struct {
	ID            int
	Name          string
	RemoteShareID string
	ItemType      ItemType
	ShareWith     string
	Owner         string
	Initiator     string
	Ctime         int
	Mtime         int
	Expiration    sql.NullInt64
	Type          ShareType
	State         ShareState
}

type dbProtocol struct {
	ID                   string
	ShareID              string
	Type                 Protocol
	WebDAVURI            *string
	WebDAVSharedSecret   *string
	WebDavPermissions    *int
	WebappURITemplate    *string
	WebappViewMode       *int
	TransferSourceURI    *string
	TransferSharedSecret *string
	TransferSize         *int
}

func convertFederatedUserID(s string) *userpb.UserId {
	split := strings.Split(s, "@")
	if len(split) < 2 {
		panic("not in the form <id>@<provider>")
	}
	return &userpb.UserId{
		OpaqueId: split[0],
		Idp:      split[1],
		Type:     userpb.UserType_USER_TYPE_FEDERATED,
	}
}

func convertToCS3OCMShare(s *dbShare, am []*ocm.AccessMethod) *ocm.Share {
	share := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: strconv.Itoa(s.ID),
		},
		ResourceId: &provider.ResourceId{
			StorageId: s.Prefix,
			OpaqueId:  s.ItemSource,
		},
		Name:  s.Name,
		Token: s.Token,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: convertFederatedUserID(s.ShareWith),
			},
		},
		Owner: &userpb.UserId{
			OpaqueId: s.Owner,
		},
		Creator: &userpb.UserId{
			OpaqueId: s.Initiator,
		},
		Ctime: &types.Timestamp{
			Seconds: uint64(s.Ctime),
		},
		Mtime: &types.Timestamp{
			Seconds: uint64(s.Mtime),
		},
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		AccessMethods: am,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.Int64),
		}
	}
	return share
}

func convertToCS3OCMReceivedShare(s *dbReceivedShare, p []*ocm.Protocol) *ocm.ReceivedShare {
	share := &ocm.ReceivedShare{
		Id: &ocm.ShareId{
			OpaqueId: strconv.Itoa(s.ID),
		},
		RemoteShareId: s.RemoteShareID,
		Name:          s.Name,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: s.ShareWith,
				},
			},
		},
		Owner:   convertFederatedUserID(s.Owner),
		Creator: convertFederatedUserID(s.Initiator),
		Ctime: &types.Timestamp{
			Seconds: uint64(s.Ctime),
		},
		Mtime: &types.Timestamp{
			Seconds: uint64(s.Mtime),
		},
		ResourceType: convertToCS3ResourceType(s.ItemType),
		ShareType:    ocm.ShareType_SHARE_TYPE_USER,
		State:        convertToCS3OCMShareState(s.State),
		Protocols:    p,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.Int64),
		}
	}
	return share
}

func convertToCS3AccessMethod(m *dbAccessMethod) *ocm.AccessMethod {
	switch m.Type {
	case WebDAVAccessMethod:
		return share.NewWebDavAccessMethod(conversions.RoleFromOCSPermissions(conversions.Permissions(*m.WebDAVPermissions)).CS3ResourcePermissions())
	case WebappAccessMethod:
		return share.NewWebappAccessMethod(appprovider.ViewMode(*m.WebAppViewMode))
	case TransferAccessMethod:
		return share.NewTransferAccessMethod()
	}
	return nil
}

func convertToCS3Protocol(p *dbProtocol) *ocm.Protocol {
	switch p.Type {
	case WebDAVProtocol:
		return share.NewWebDAVProtocol(*p.WebDAVURI, *p.WebDAVSharedSecret, &ocm.SharePermissions{
			Permissions: conversions.RoleFromOCSPermissions(conversions.Permissions(*p.WebDavPermissions)).CS3ResourcePermissions(),
		})
	case WebappProtocol:
		return share.NewWebappProtocol(*p.WebappURITemplate, appprovider.ViewMode(*p.WebappViewMode))
	case TransferProtocol:
		return share.NewTransferProtocol(*p.TransferSourceURI, *p.TransferSharedSecret, uint64(*p.TransferSize))
	}
	return nil
}

func convertToCS3ResourceType(t ItemType) provider.ResourceType {
	switch t {
	case ItemTypeFile:
		return provider.ResourceType_RESOURCE_TYPE_FILE
	case ItemTypeFolder:
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_INVALID
}

func convertFromCS3ResourceType(t provider.ResourceType) ItemType {
	switch t {
	case provider.ResourceType_RESOURCE_TYPE_FILE:
		return ItemTypeFile
	case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		return ItemTypeFolder
	}
	return -1
}
