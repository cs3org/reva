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
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ItemType string

const (
	ItemTypeFile      ItemType = "file"
	ItemTypeFolder    ItemType = "folder"
	ItemTypeReference ItemType = "reference"
	ItemTypeSymlink   ItemType = "symlink"
)

func (i ItemType) String() string {
	return string(i)
}

// ShareID only contains IDs of shares and public links. This is because the Web UI requires
// that shares and public links do not share an ID, so we need a shared table to make sure
// that there are no duplicates.
// This is implemented by having ShareID have an ID that is auto-increment, and shares and
// public links will have their ID be a foreign key to ShareID
// When creating a new share, we will then first create an ID entry and use this for the ID

type ShareID struct {
	ID uint `gorm:"primarykey"`
}

// We cannot use gorm.Model, because we want our ID to be a foreign key to ShareID
type BaseModel struct {
	// Id has to be called Id and not ID, otherwise the foreign key will not work
	// ID is a special field in GORM, which it uses as the default Primary Key
	Id        uint    `gorm:"primaryKey;not null;autoIncrement:false"`
	ShareId   ShareID `gorm:"foreignKey:Id;references:ID;constraint:OnDelete:CASCADE"` //;references:ID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ProtoShare contains fields that are shared between PublicLinks and Shares.
// Unfortunately, because these are shared, we cannot name our indexes
// because then two indexes with the same name would be created
type ProtoShare struct {
	// Including gorm.Model will embed a number of gorm-default fields
	BaseModel
	UIDOwner     string   `gorm:"size:64"`
	UIDInitiator string   `gorm:"size:64;index"`
	ItemType     ItemType `gorm:"size:16;index"` // file | folder | reference | symlink
	InitialPath  string
	Inode        string `gorm:"size:32;index"`
	Instance     string `gorm:"size:32;index"`
	Permissions  uint8
	Orphan       bool
	Expiration   datatypes.NullTime
}

type Share struct {
	ProtoShare
	ShareWith         string `gorm:"size:255;index:i_share_with"` // 255 because this can be a lw account, which are mapped from email addresses / ...
	SharedWithIsGroup bool
	Description       string `gorm:"size:1024"`
}

type PublicLink struct {
	ProtoShare
	// Current tokens are only 16 chars long, but old tokens used to be 32 characters
	Token                        string `gorm:"uniqueIndex:i_token;size:32"`
	Quicklink                    bool
	NotifyUploads                bool
	NotifyUploadsExtraRecipients string
	Password                     string `gorm:"size:255"`
	// Users can give a name to a share
	LinkName string `gorm:"size:512"`
}

type ShareState struct {
	gorm.Model
	ShareID uint  `gorm:"uniqueIndex:i_shareid_user"`       // Define the foreign key field
	Share   Share `gorm:"foreignKey:ShareID;references:Id"` // Define the association
	// Can not be uid because of lw accs
	User   string `gorm:"uniqueIndex:i_shareid_user;size:255"`
	Synced bool
	Hidden bool
	Alias  string `gorm:"size:64"`
}

func (s *Share) AsCS3Share(granteeType userpb.UserType) *collaboration.Share {
	creationTs := &typespb.Timestamp{
		Seconds: uint64(s.CreatedAt.Unix()),
	}
	updateTs := &typespb.Timestamp{
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
		Permissions: &collaboration.SharePermissions{Permissions: conversions.IntTosharePerm(int(s.Permissions), s.ItemType.String())},
		Grantee:     extractGrantee(s.SharedWithIsGroup, s.ShareWith, granteeType),
		Owner:       conversions.MakeUserID(s.UIDOwner),
		Creator:     conversions.MakeUserID(s.UIDInitiator),
		Ctime:       creationTs,
		Mtime:       updateTs,
		Description: s.Description,
	}

	if s.Expiration.Valid {
		share.Expiration = &typespb.Timestamp{
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
	ts := &typespb.Timestamp{
		Seconds: uint64(p.CreatedAt.Unix()),
	}

	var expires *typespb.Timestamp
	if p.Expiration.Valid {
		exp, err := p.Expiration.Value()
		if err == nil {
			expiration := exp.(time.Time)
			expires = &typespb.Timestamp{
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
		Permissions:                  &link.PublicSharePermissions{Permissions: conversions.IntTosharePerm(int(p.Permissions), p.ItemType.String())},
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
