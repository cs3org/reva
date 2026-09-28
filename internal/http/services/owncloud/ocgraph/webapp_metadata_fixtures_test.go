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
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

const (
	receivedWebappFixtureSubdir        = "testdata/received-webapp"
	receivedWebappManifestSHA256       = "2a8d0b6dbc82b2478aa53abefdf7864b1c016d261b345f4a6ebfcde852f3cbb1"
	receivedWebappExpectationsRevision = "received-webapp-v1-2026-09-28"
)

var receivedWebappFixtureJSONFiles = []string{
	"empty-name.sharedWithMe.json",
	"positive.sharedWithMe.json",
	"resource-expectations.json",
	"webdav-only.sharedWithMe.json",
}

type receivedWebappFixtures struct {
	Positive     []byte
	WebDAVOnly   []byte
	EmptyName    []byte
	Expectations receivedWebappResourceExpectations
	Sums         map[string]string
}

type receivedWebappResourceExpectations struct {
	Revision string `json:"revision"`
	Cases    []struct {
		Response string `json:"response"`
	} `json:"cases"`
}

func loadReceivedWebappFixturesStrict(t *testing.T) receivedWebappFixtures {
	t.Helper()

	dir := receivedWebappFixtureSubdir
	manifestPath := filepath.Join(dir, "SHA256SUMS")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	if got := sha256Hex(manifestBytes); got != receivedWebappManifestSHA256 {
		t.Fatalf("SHA256SUMS digest = %s, want %s", got, receivedWebappManifestSHA256)
	}

	sums, err := parseReceivedWebappSHA256SUMS(manifestBytes)
	if err != nil {
		t.Fatalf("parse SHA256SUMS: %v", err)
	}
	if len(sums) != len(receivedWebappFixtureJSONFiles) {
		t.Fatalf("SHA256SUMS entries = %d, want %d", len(sums), len(receivedWebappFixtureJSONFiles))
	}

	readFixture := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		wantHash, ok := sums[name]
		if !ok {
			t.Fatalf("SHA256SUMS missing %s", name)
		}
		if got := sha256Hex(raw); got != wantHash {
			t.Fatalf("%s digest = %s, want %s", name, got, wantHash)
		}
		return raw
	}

	for _, name := range receivedWebappFixtureJSONFiles {
		if _, ok := sums[name]; !ok {
			t.Fatalf("SHA256SUMS missing required file %s", name)
		}
	}

	positive := readFixture("positive.sharedWithMe.json")
	webdavOnly := readFixture("webdav-only.sharedWithMe.json")
	emptyName := readFixture("empty-name.sharedWithMe.json")
	expectationsRaw := readFixture("resource-expectations.json")

	var expectations receivedWebappResourceExpectations
	if err := json.Unmarshal(expectationsRaw, &expectations); err != nil {
		t.Fatalf("decode resource-expectations.json: %v", err)
	}

	return receivedWebappFixtures{
		Positive:     positive,
		WebDAVOnly:   webdavOnly,
		EmptyName:    emptyName,
		Expectations: expectations,
		Sums:         sums,
	}
}

func parseReceivedWebappSHA256SUMS(raw []byte) (map[string]string, error) {
	sums := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, errors.New("malformed SHA256SUMS line: " + line)
		}
		sums[fields[1]] = fields[0]
	}
	return sums, nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func goldenReceivedShareDriveItem(t *testing.T, webdavOnly []byte) *libregraph.DriveItem {
	t.Helper()
	raw := decodeSharedWithMeItem(t, webdavOnly)
	var item libregraph.DriveItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("decode golden drive item: %v", err)
	}
	return &item
}

func assertSharedWithMeEnvelopeEqual(t *testing.T, got, want []byte) {
	t.Helper()
	if !jsonSemanticEqual(t, got, want) {
		t.Fatalf("sharedWithMe envelope differs\n got %s\nwant %s", got, want)
	}
}

func jsonSemanticEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	normalize := func(raw []byte) []byte {
		t.Helper()
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode json: %v; raw %s", err, raw)
		}
		out, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	return bytes.Equal(normalize(a), normalize(b))
}

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

func receivedWebappContractShare(present bool, appName string) *ocm.ReceivedShare {
	const folderName = "shared-folder"
	share := receivedOCMShareInfo()
	share.Name = folderName
	share.SharedResourceType = ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
	share.Protocols[0].GetWebdavOptions().Uri = "https://remote.example/dav/" + folderName
	if !present {
		return share
	}
	share.Protocols[0].GetWebdavOptions().SharedSecret = "webdav-secret-must-stay-in-storage"
	share.Protocols = append(share.Protocols, &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				Uri:          "https://remote.example/open",
				SharedSecret: "secret-must-stay-in-storage",
				AppName:      appName,
				AppIconHint:  "image/png",
				MediaTypes:   []string{"text/markdown"},
				Requirements: []string{"must-exchange-token"},
				Targets:      []string{"blank"},
			},
		},
	})
	return share
}
