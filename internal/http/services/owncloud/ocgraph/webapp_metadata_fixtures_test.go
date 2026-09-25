// Copyright 2018-2024 CERN
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

package ocgraph

import (
	"encoding/base32"
	"errors"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	libregraph "github.com/owncloud/libre-graph-api-go"

	"github.com/cs3org/reva/v3/pkg/permissions"
)

type failJSON struct{}

func (failJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("marshal failed")
}

func sparseOptionalDriveItem() *libregraph.DriveItem {
	item := webdavOnlyDriveItem()
	item.Description = nil
	item.ClientSynchronize = nil
	item.UIHidden = nil
	item.File = nil
	item.Size = nil
	item.ETag = nil
	item.WebDavUrl = nil
	item.LastModifiedDateTime = nil
	item.RemoteItem.File = nil
	item.RemoteItem.ETag = nil
	item.RemoteItem.Size = nil
	item.RemoteItem.Path = nil
	item.RemoteItem.WebUrl = nil
	item.RemoteItem.WebDavUrl = nil
	item.RemoteItem.LastModifiedDateTime = nil
	return item
}

func webdavOnlyDriveItem() *libregraph.DriveItem {
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	grant := libregraph.Permission{
		Id:              libregraph.PtrString("grant-1"),
		Roles:           []string{"b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"},
		CreatedDateTime: *libregraph.NewNullableTime(&created),
		GrantedToV2: &libregraph.SharePointIdentitySet{
			User: &libregraph.Identity{
				Id:          libregraph.PtrString("grantee"),
				DisplayName: "Grantee",
			},
		},
		Invitation: &libregraph.SharingInvitation{
			InvitedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					Id:          libregraph.PtrString("creator"),
					DisplayName: "Creator",
				},
			},
		},
	}
	return &libregraph.DriveItem{
		Id:                   libregraph.PtrString("item-1"),
		Name:                 libregraph.PtrString("notes.txt"),
		ETag:                 libregraph.PtrString("etag-1"),
		Size:                 libregraph.PtrInt64(12),
		WebDavUrl:            libregraph.PtrString("https://example.test/dav/notes.txt"),
		Description:          libregraph.PtrString("optional description"),
		ClientSynchronize:    libregraph.PtrBool(true),
		UIHidden:             libregraph.PtrBool(false),
		LastModifiedDateTime: libregraph.PtrTime(created),
		File: &libregraph.OpenGraphFile{
			MimeType: libregraph.PtrString("text/plain"),
		},
		RemoteItem: &libregraph.RemoteItem{
			Id:                   libregraph.PtrString("remote-1"),
			Name:                 libregraph.PtrString("notes.txt"),
			ETag:                 libregraph.PtrString("etag-1"),
			Size:                 libregraph.PtrInt64(12),
			Path:                 libregraph.PtrString("notes.txt"),
			WebUrl:               libregraph.PtrString("https://example.test/files/notes.txt"),
			WebDavUrl:            libregraph.PtrString("https://example.test/dav/notes.txt"),
			LastModifiedDateTime: libregraph.PtrTime(created),
			Permissions:          []libregraph.Permission{grant},
		},
	}
}

func secondGrant() libregraph.Permission {
	created := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)
	return libregraph.Permission{
		Id:              libregraph.PtrString("grant-2"),
		Roles:           []string{"2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"},
		CreatedDateTime: *libregraph.NewNullableTime(&created),
		GrantedToV2: &libregraph.SharePointIdentitySet{
			User: &libregraph.Identity{
				Id:          libregraph.PtrString("other"),
				DisplayName: "Other",
			},
		},
		Invitation: &libregraph.SharingInvitation{
			InvitedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					Id:          libregraph.PtrString("creator"),
					DisplayName: "Creator",
				},
			},
		},
	}
}

func receivedShareInfo() *gateway.ReceivedShareResourceInfo {
	spaceID := base32.StdEncoding.EncodeToString([]byte("/users/alice"))
	created := &types.Timestamp{Seconds: 1700000000}
	return &gateway.ReceivedShareResourceInfo{
		ReceivedShare: &collaboration.ReceivedShare{
			Share: &collaboration.Share{
				Id:      &collaboration.ShareId{OpaqueId: "share-1"},
				Creator: &userpb.UserId{OpaqueId: "creator"},
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{OpaqueId: "grantee"},
					},
				},
				Ctime: created,
			},
		},
		ResourceInfo: &provider.ResourceInfo{
			Id: &provider.ResourceId{
				StorageId: "storage",
				OpaqueId:  "opaque",
				SpaceId:   spaceID,
			},
			Path:          "/users/alice/readme.txt",
			Name:          "readme.txt",
			Etag:          "etag",
			MimeType:      "text/plain",
			Size:          4,
			Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
			PermissionSet: permissions.NewViewerRole().CS3ResourcePermissions(),
			Mtime:         created,
		},
	}
}

func receivedOCMShareInfo() *ocm.ReceivedShare {
	created := &types.Timestamp{Seconds: 1700000000}
	return &ocm.ReceivedShare{
		Id:   &ocm.ShareId{OpaqueId: "ocm-share-1"},
		Name: "remote.txt",
		Creator: &userpb.UserId{
			OpaqueId: "remote-creator",
			Idp:      "remote.example",
		},
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: "me",
					Idp:      "local.example",
				},
			},
		},
		Ctime:              created,
		Mtime:              created,
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		Protocols: []*ocm.Protocol{
			{
				Term: &ocm.Protocol_WebdavOptions{
					WebdavOptions: &ocm.WebDAVProtocol{
						Uri: "https://remote.example/dav/remote.txt",
						Permissions: &ocm.SharePermissions{
							Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
						},
					},
				},
			},
		},
	}
}
