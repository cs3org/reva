// Copyright 2018-2025 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

package model

import (
	"strconv"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	resourcespb "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"
	"github.com/cs3org/reva/v3/pkg/permissions"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ItemType is the type of the shared resource.
type ItemType string

const (
	ItemTypeFile      ItemType = "file"
	ItemTypeFolder    ItemType = "folder"
	ItemTypeReference ItemType = "reference"
	ItemTypeSymlink   ItemType = "symlink"
	ItemTypeEmbedded  ItemType = "embedded"
)

func (i ItemType) String() string {
	return string(i)
}

// For OCM shares, OcmShareType is the type of the recipient.
type OcmShareType int

const (
	// OcmShareTypeUser is used for a share to an user.
	OcmShareTypeUser OcmShareType = iota
	// OcmShareTypeGroup is used for a share to a group.
	OcmShareTypeGroup
)

// For OCM shares, OcmShareState is their state.
type OcmShareState int

const (
	// OcmShareStatePending is the state for a pending share.
	OcmShareStatePending OcmShareState = iota
	// OcmShareStateAccepted is the share for an accepted share.
	OcmShareStateAccepted
	// OcmShareStateRejected is the share for a rejected share.
	OcmShareStateRejected
)

// OcmProtocol is the protocol used by the recipient of an OCM share
// (both incoming and outgoing) to access the shared resource.
type OcmProtocol int

const (
	// WebDAVProtocol is the OCM `webdav` protocol.
	WebDAVProtocol OcmProtocol = iota
	// WebappProtocol is the OCM `webapp` protocol.
	WebappProtocol
	// EmbeddedProtocol is the OCM `embedded` protocol.
	EmbeddedProtocol
	// SshProtocol is the OCM `ssh` protocol (not currently supported).
	SshProtocol
)

// OcmAccessType represents the access type to be used in OCM shares:
// currently supported values are `remote` and `datatx`, possibly
// combined as a bitmask.
type OcmAccessType int32

const (
	// AccessTypeRemote is the OCM `remote` access type.
	AccessTypeRemote OcmAccessType = 1
	// AccessTypeDataTx is the OCM `datatx` access type.
	AccessTypeDataTx OcmAccessType = 2
	// AccessTypeBoth is the OCM `remote+datatx` access type.
	AccessTypeBoth OcmAccessType = 3
)

// ShareID only contains IDs of shares and public links. This is because the Web UI requires
// that shares and public links do not share an ID, so we need a shared table to make sure
// that there are no duplicates.
// This is implemented by having ShareID have an ID that is auto-increment, and shares and
// public links will have their ID be a foreign key to ShareID
// When creating a new share, we will then first create an ID entry and use this for the ID
type ShareID struct {
	ID uint `gorm:"primarykey"`
}

// This is the base model for all share types and embeds parts of gorm.Model. We can't use it, because
// we want our ID to be a foreign key to ShareID, but we incorporate the date fields from GORM.
// The commented-out fields would logically belong here, but we define them in each specific type
// to control the unique indexes to be enforced in the corresponding tables.
type ProtoShare struct {
	// Id has to be called Id and not ID, otherwise the foreign key will not work
	// ID is a special field in GORM, which it uses as the default Primary Key
	Id        uint    `gorm:"primaryKey;not null;autoIncrement:false"`
	ShareId   ShareID `gorm:"foreignKey:Id;references:ID;constraint:OnDelete:CASCADE"` //;references:ID
	CreatedAt time.Time
	UpdatedAt time.Time
	//DeletedAt  gorm.DeletedAt `gorm:"index"`
	//Inode      string `gorm:"size:32;index"`
	//Instance   string `gorm:"size:32;index"`
	UIDOwner     string   `gorm:"size:64;index"`
	UIDInitiator string   `gorm:"size:64;index"`
	ItemType     ItemType `gorm:"size:16;index"` // file | folder | reference | symlink
	InitialPath  string
	Permissions  uint8
	Orphan       bool               `gorm:"index"`
	Expiration   datatypes.NullTime `gorm:"index"`
}

// Share is a regular share between users or groups. The unique index ensures that there
// can only be one share per (inode, instance, recipient) tuple, unless the share is deleted.
type Share struct {
	ProtoShare
	// Note that the order of the fields here determines the order of the composite index u_share.
	// `deleted_at` has no separate index here because it is the first entry in the composite index,
	// which MySQL can use.
	// Should you change the order of the fields, make sure to add an index to `deleted_at`, or
	// you might encounter performance issues!
	DeletedAt         gorm.DeletedAt `gorm:"uniqueIndex:u_share"`
	Inode             string         `gorm:"size:32;uniqueIndex:u_share;index"`
	Instance          string         `gorm:"size:32;uniqueIndex:u_share;index"`
	ShareWith         string         `gorm:"size:255;uniqueIndex:u_share;index"` // 255 because this can be an external account, which has a long representation
	SharedWithIsGroup bool
	Description       string `gorm:"size:1024"`
}

// PublicLink is a public link share. We only enforce a unique constraint on the token.
type PublicLink struct {
	ProtoShare
	DeletedAt                    gorm.DeletedAt `gorm:"index"`
	Inode                        string         `gorm:"size:32;index"`
	Instance                     string         `gorm:"size:32;index"`
	Token                        string         `gorm:"uniqueIndex:u_link_token;size:32;index"` // Current tokens are only 16 chars long, but old tokens used to be 32 characters
	Quicklink                    bool
	NotifyUploads                bool
	NotifyUploadsExtraRecipients string
	Password                     string `gorm:"size:255"`
	LinkName                     string `gorm:"size:512"` // Users can give a name to a share
}

// ShareState represents the state of a share for a specific recipient.
type ShareState struct {
	gorm.Model
	ShareID uint   `gorm:"uniqueIndex:u_shareid_user"`          // Define the foreign key field
	Share   Share  `gorm:"foreignKey:ShareID;references:Id"`    // Define the association
	User    string `gorm:"uniqueIndex:u_shareid_user;size:255"` // Can not be uid because of lw accounts
	Synced  bool
	Hidden  bool
	Alias   string `gorm:"size:64"`
}

// OcmShare represents an OCM share for a remote user. The unique index ensures that there
// can only be one share per (inode, instance, recipient) tuple, unless the share is deleted.
// In addition, tokens must be unique.
// TODO(lopresti) see if we can consolidate Owner and Initiator with UIDOwner and UIDInitiator in ProtoShare
type OcmShare struct {
	ProtoShare
	DeletedAt     gorm.DeletedAt     `gorm:"uniqueIndex:u_ocmshare"` // the composite index also works as regular index here, similarly as in the `Share` struct
	Inode         string             `gorm:"size:64;not null;uniqueIndex:u_ocmshare;index"`
	Instance      string             `gorm:"size:64;not null;uniqueIndex:u_ocmshare;index"`
	Token         string             `gorm:"size:255;not null;uniqueIndex:u_ocmshare_token"`
	Name          string             `gorm:"type:text;not null"`
	ShareWith     string             `gorm:"size:255;not null;uniqueIndex:u_ocmshare;index"`
	Owner         string             `gorm:"size:255;not null"`
	Initiator     string             `gorm:"type:text;not null"`
	Ctime         uint64             `gorm:"not null"`
	Mtime         uint64             `gorm:"not null"`
	RecipientType OcmShareType       `gorm:"not null"`
	Protocols     []OcmShareProtocol `gorm:"constraint:OnDelete:CASCADE;"`
}

// OcmShareProtocol represents the protocol used to serve an OCM share, named AccessMethod in the OCM CS3 APIs.
type OcmShareProtocol struct {
	gorm.Model
	OcmShareID  uint          `gorm:"not null;uniqueIndex:u_ocm_share_protocol"`
	Type        OcmProtocol   `gorm:"not null;uniqueIndex:u_ocm_share_protocol"`
	Permissions int           `gorm:"default:null"`
	AccessTypes OcmAccessType `gorm:"default:null"`
}

// OcmReceivedShare represents an OCM share received from a remote user.
type OcmReceivedShare struct {
	gorm.Model
	RemoteShareID string             `gorm:"index:i_ocmrecshare_remoteshareid;not null"`
	Name          string             `gorm:"size:255;not null"`
	ItemType      ItemType           `gorm:"size:16;not null"`
	ShareWith     string             `gorm:"size:255;not null"`
	Owner         string             `gorm:"index:i_ocmrecshare_owner;size:255;not null"`
	Initiator     string             `gorm:"index:i_ocmrecshare_initiator;size:255;not null"`
	Ctime         uint64             `gorm:"not null"`
	Mtime         uint64             `gorm:"not null"`
	Expiration    datatypes.NullTime `gorm:"index"`
	RecipientType OcmShareType       `gorm:"index:i_ocmrecshare_type;not null"`
	State         OcmShareState      `gorm:"index:i_ocmrecshare_state;not null"`
	Alias         string             `gorm:"size:64"`
	Hidden        bool
}

// OcmReceivedShareProtocol represents the protocol used to access an OCM share received from a remote user.
type OcmReceivedShareProtocol struct {
	gorm.Model
	OcmReceivedShareID uint             `gorm:"not null;uniqueIndex:u_ocmrecshare_protocol"`
	OcmReceivedShare   OcmReceivedShare `gorm:"constraint:OnDelete:CASCADE;foreignKey:OcmReceivedShareID;references:ID"`
	Type               OcmProtocol      `gorm:"not null;uniqueIndex:u_ocmrecshare_protocol"`
	Uri                string           `gorm:"size:255"`
	SharedSecret       string           `gorm:"type:text;not null"`
	// WebDAV and WebApp Protocol fields
	Permissions int           `gorm:"default:null"`
	AccessTypes OcmAccessType `gorm:"default:null"`
	// JSON field for the embedded protocol payload
	Payload datatypes.JSON `gorm:"type:json;default:null"`
}

func (s *Share) AsCS3Share(granteeType userpb.UserType) *collaboration.Share {
	creationTs := &types.Timestamp{
		Seconds: uint64(s.CreatedAt.Unix()),
	}
	updateTs := &types.Timestamp{
		Seconds: uint64(s.UpdatedAt.Unix()),
	}
	share := &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: strconv.FormatUint(uint64(s.Id), 10),
		},
		//ResourceId:  &provider.Reference{StorageId: s.Prefix, NodeId: s.ItemSource},
		ResourceId: &provider.ResourceId{
			StorageId: s.Instance,
			OpaqueId:  s.Inode,
		},
		Permissions: &collaboration.SharePermissions{Permissions: permissions.OcsPermissions(s.Permissions).AsCS3Permissions()},
		Grantee:     extractGrantee(s.SharedWithIsGroup, s.ShareWith, granteeType),
		Owner:       conversions.MakeUserID(s.UIDOwner),
		Creator:     conversions.MakeUserID(s.UIDInitiator),
		Ctime:       creationTs,
		Mtime:       updateTs,
		Description: s.Description,
	}

	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.V.Unix()),
		}
	}

	return share
}

func (s *Share) AsCS3ReceivedShare(state *ShareState, granteeType userpb.UserType) *collaboration.ReceivedShare {
	// Currently, some implementations still rely on the ShareState to determine whether a file is hidden
	// instead of using the field
	var rsharestate resourcespb.ShareState
	if state.Hidden {
		rsharestate = resourcespb.ShareState_SHARE_STATE_REJECTED
	} else {
		rsharestate = resourcespb.ShareState_SHARE_STATE_ACCEPTED
	}

	return &collaboration.ReceivedShare{
		Share:  s.AsCS3Share(granteeType),
		State:  rsharestate,
		Hidden: state.Hidden,
		Alias:  state.Alias,
	}
}

func (p *PublicLink) AsCS3PublicShare() *link.PublicShare {
	ts := &types.Timestamp{
		Seconds: uint64(p.CreatedAt.Unix()),
	}

	var expires *types.Timestamp
	if p.Expiration.Valid {
		exp, err := p.Expiration.Value()
		if err == nil {
			expiration := exp.(time.Time)
			expires = &types.Timestamp{
				Seconds: uint64(expiration.Unix()),
			}
		}

	}
	return &link.PublicShare{
		Id: &link.PublicShareId{
			OpaqueId: strconv.Itoa(int(p.Id)),
		},
		ResourceId: &provider.ResourceId{
			StorageId: p.Instance,
			OpaqueId:  p.Inode,
		},
		Permissions:                  &link.PublicSharePermissions{Permissions: permissions.OcsPermissions(p.Permissions).AsCS3Permissions()},
		Owner:                        conversions.MakeUserID(p.UIDOwner),
		Creator:                      conversions.MakeUserID(p.UIDInitiator),
		Token:                        p.Token,
		DisplayName:                  defaultLinkDisplayName(p.LinkName, p.Quicklink),
		PasswordProtected:            p.Password != "",
		Expiration:                   expires,
		Ctime:                        ts,
		Mtime:                        ts,
		Quicklink:                    p.Quicklink,
		NotifyUploads:                p.NotifyUploads,
		NotifyUploadsExtraRecipients: p.NotifyUploadsExtraRecipients,
	}
}

// ExtractGrantee retrieves the CS3API Grantee from a grantee type and username/groupname.
// The grantee userType is relevant only for users.
func extractGrantee(sharedWithIsGroup bool, g string, gtype userpb.UserType) *provider.Grantee {
	var grantee provider.Grantee
	if sharedWithIsGroup {
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_GROUP
		grantee.Id = &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{
			OpaqueId: g,
		}}
	} else {
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_USER
		grantee.Id = &provider.Grantee_UserId{UserId: &userpb.UserId{
			OpaqueId: g,
			Type:     gtype,
		}}
	}
	return &grantee
}

func defaultLinkDisplayName(displayName string, quickLink bool) string {
	if displayName != "" {
		return displayName
	} else if quickLink {
		return "QuickLink"
	} else {
		return "Link"
	}
}
