// Copyright 2018-2021 CERN
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

package nextcloud_test

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"

	"github.com/cs3org/reva/pkg/share/manager/nextcloud"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func setUpNextcloudServer() (*nextcloud.Manager, *[]string, func()) {
	var conf *nextcloud.ShareManagerConfig

	ncHost := os.Getenv("NEXTCLOUD")
	fmt.Printf(`NEXTCLOUD env var: "%s"`, ncHost)
	if len(ncHost) == 0 {
		conf = &nextcloud.ShareManagerConfig{
			EndPoint: "http://mock.com/apps/sciencemesh/",
			MockHTTP: true,
		}
		nc, _ := nextcloud.NewShareManager(conf)
		called := make([]string, 0)
		h := nextcloud.GetNextcloudServerMock(&called)
		mock, teardown := nextcloud.TestingHTTPClient(h)
		nc.SetHTTPClient(mock)
		return nc, &called, teardown
	} else {
		conf = &nextcloud.ShareManagerConfig{
			EndPoint: ncHost + "/apps/sciencemesh/",
			MockHTTP: false,
		}
		nc, _ := nextcloud.NewShareManager(conf)
		return nc, nil, func() {}
	}
}

func checkCalled(called *[]string, expected string) {
	if called == nil {
		return
	}
	Expect(len(*called)).To(Equal(1))
	Expect((*called)[0]).To(Equal(expected))
}

var _ = Describe("Nextcloud", func() {
	var (
		ctx     context.Context
		options map[string]interface{}
		tmpRoot string
		user    = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "tester",
		}
	)

	BeforeEach(func() {
		var err error
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"root":         tmpRoot,
			"enable_home":  true,
			"share_folder": "/Shares",
		}

		ctx = context.Background()

		// Add auth token
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		scope, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user, scope)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, user)
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			_, err := nextcloud.New(options)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error)
	FDescribe("Share", func() {
		It("calls the Share endpoint", func() {
			fmt.Println("Calling setUpNextCloudServer!")
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			share, err := am.Share(ctx, &provider.ResourceInfo{
				Opaque: &types.Opaque{
					Map:                  nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Type: provider.ResourceType_RESOURCE_TYPE_FILE,
				Id: &provider.ResourceId{
					StorageId:            "",
					OpaqueId:             "fileid-/some/path",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Checksum: &provider.ResourceChecksum{
					Type:                 0,
					Sum:                  "",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Etag:     "deadbeef",
				MimeType: "text/plain",
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Path: "/some/path",
				PermissionSet: &provider.ResourcePermissions{
					AddGrant:             false,
					CreateContainer:      false,
					Delete:               false,
					GetPath:              false,
					GetQuota:             false,
					InitiateFileDownload: false,
					InitiateFileUpload:   false,
					ListGrants:           false,
					ListContainer:        false,
					ListFileVersions:     false,
					ListRecycle:          false,
					Move:                 false,
					RemoveGrant:          false,
					PurgeRecycle:         false,
					RestoreFileVersion:   false,
					RestoreRecycleItem:   false,
					Stat:                 false,
					UpdateGrant:          false,
					DenyGrant:            false,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Size:   12345,
				Owner:  nil,
				Target: "",
				CanonicalMetadata: &provider.CanonicalMetadata{
					Target:               nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				ArbitraryMetadata: &provider.ArbitraryMetadata{
					Metadata:             map[string]string{"some": "arbi", "trary": "meta", "da": "ta"},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}, &collaboration.ShareGrant{
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*share).To(Equal(collaboration.Share{
				Id:         &collaboration.ShareId{},
				ResourceId: &provider.ResourceId{},
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						AddGrant:             true,
						CreateContainer:      true,
						Delete:               true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
						ListGrants:           true,
						ListContainer:        true,
						ListFileVersions:     true,
						ListRecycle:          true,
						Move:                 true,
						RemoveGrant:          true,
						PurgeRecycle:         true,
						RestoreFileVersion:   true,
						RestoreRecycleItem:   true,
						Stat:                 true,
						UpdateGrant:          true,
						DenyGrant:            true,
					},
				},
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Ctime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/Share {"md":{"opaque":{},"type":1,"id":{"opaque_id":"fileid-/some/path"},"checksum":{},"etag":"deadbeef","mime_type":"text/plain","mtime":{"seconds":1234567890},"path":"/some/path","permission_set":{},"size":12345,"canonical_metadata":{},"arbitrary_metadata":{"metadata":{"da":"ta","some":"arbi","trary":"meta"}}},"g":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"permissions":{}}}}`)
		})
	})

	// GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error)
	Describe("GetShare", func() {
		It("calls the GetShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			share, err := am.GetShare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*share).To(Equal(collaboration.Share{
				Id:         &collaboration.ShareId{},
				ResourceId: &provider.ResourceId{},
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						AddGrant:             true,
						CreateContainer:      true,
						Delete:               true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
						ListGrants:           true,
						ListContainer:        true,
						ListFileVersions:     true,
						ListRecycle:          true,
						Move:                 true,
						RemoveGrant:          true,
						PurgeRecycle:         true,
						RestoreFileVersion:   true,
						RestoreRecycleItem:   true,
						Stat:                 true,
						UpdateGrant:          true,
						DenyGrant:            true,
					},
				},
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Ctime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/GetShare {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// Unshare(ctx context.Context, ref *collaboration.ShareReference) error
	Describe("Unshare", func() {
		It("calls the Unshare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			err := am.Unshare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/Unshare {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error)
	Describe("UpdateShare", func() {
		It("calls the UpdateShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			share, err := am.UpdateShare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			},
				&collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						AddGrant:             true,
						CreateContainer:      true,
						Delete:               true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
						ListGrants:           true,
						ListContainer:        true,
						ListFileVersions:     true,
						ListRecycle:          true,
						Move:                 true,
						RemoveGrant:          true,
						PurgeRecycle:         true,
						RestoreFileVersion:   true,
						RestoreRecycleItem:   true,
						Stat:                 true,
						UpdateGrant:          true,
						DenyGrant:            true,
					},
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(*share).To(Equal(collaboration.Share{
				Id:         &collaboration.ShareId{},
				ResourceId: &provider.ResourceId{},
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						AddGrant:             true,
						CreateContainer:      true,
						Delete:               true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
						ListGrants:           true,
						ListContainer:        true,
						ListFileVersions:     true,
						ListRecycle:          true,
						Move:                 true,
						RemoveGrant:          true,
						PurgeRecycle:         true,
						RestoreFileVersion:   true,
						RestoreRecycleItem:   true,
						Stat:                 true,
						UpdateGrant:          true,
						DenyGrant:            true,
					},
				},
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Ctime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/UpdateShare {"ref":{"Spec":{"Id":{"opaque_id":"some-share-id"}}},"p":{"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`)
		})
	})

	// ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error)
	Describe("ListShares", func() {
		It("calls the ListShares endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			shares, err := am.ListShares(ctx, []*collaboration.Filter{
				{
					Type: collaboration.Filter_TYPE_CREATOR,
					Term: &collaboration.Filter_Creator{
						Creator: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(1))
			Expect(*shares[0]).To(Equal(collaboration.Share{
				Id:         &collaboration.ShareId{},
				ResourceId: &provider.ResourceId{},
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						AddGrant:             true,
						CreateContainer:      true,
						Delete:               true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
						ListGrants:           true,
						ListContainer:        true,
						ListFileVersions:     true,
						ListRecycle:          true,
						Move:                 true,
						RemoveGrant:          true,
						PurgeRecycle:         true,
						RestoreFileVersion:   true,
						RestoreRecycleItem:   true,
						Stat:                 true,
						UpdateGrant:          true,
						DenyGrant:            true,
					},
				},
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Ctime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/ListShares [{"type":4,"Term":{"Creator":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}}]`)
		})
	})

	// ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error)
	Describe("ListReceivedShares", func() {
		It("calls the ListReceivedShares endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShares, err := am.ListReceivedShares(ctx, []*collaboration.Filter{
				{
					Type: collaboration.Filter_TYPE_CREATOR,
					Term: &collaboration.Filter_Creator{
						Creator: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(receivedShares)).To(Equal(1))
			Expect(*receivedShares[0]).To(Equal(collaboration.ReceivedShare{
				Share: &collaboration.Share{
					Id:         &collaboration.ShareId{},
					ResourceId: &provider.ResourceId{},
					Permissions: &collaboration.SharePermissions{
						Permissions: &provider.ResourcePermissions{
							AddGrant:             true,
							CreateContainer:      true,
							Delete:               true,
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
							ListGrants:           true,
							ListContainer:        true,
							ListFileVersions:     true,
							ListRecycle:          true,
							Move:                 true,
							RemoveGrant:          true,
							PurgeRecycle:         true,
							RestoreFileVersion:   true,
							RestoreRecycleItem:   true,
							Stat:                 true,
							UpdateGrant:          true,
							DenyGrant:            true,
						},
					},
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: &userpb.UserId{
								Idp:      "0.0.0.0:19000",
								OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
								Type:     userpb.UserType_USER_TYPE_PRIMARY,
							},
						},
					},
					Owner: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Creator: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Ctime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
					Mtime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
				},
				State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/ListReceivedShares [{"type":4,"Term":{"Creator":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}}]`)
		})
	})

	// GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error)
	Describe("GetReceivedShare", func() {
		It("calls the GetReceivedShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShare, err := am.GetReceivedShare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*receivedShare).To(Equal(collaboration.ReceivedShare{
				Share: &collaboration.Share{
					Id:         &collaboration.ShareId{},
					ResourceId: &provider.ResourceId{},
					Permissions: &collaboration.SharePermissions{
						Permissions: &provider.ResourcePermissions{
							AddGrant:             true,
							CreateContainer:      true,
							Delete:               true,
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
							ListGrants:           true,
							ListContainer:        true,
							ListFileVersions:     true,
							ListRecycle:          true,
							Move:                 true,
							RemoveGrant:          true,
							PurgeRecycle:         true,
							RestoreFileVersion:   true,
							RestoreRecycleItem:   true,
							Stat:                 true,
							UpdateGrant:          true,
							DenyGrant:            true,
						},
					},
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: &userpb.UserId{
								Idp:      "0.0.0.0:19000",
								OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
								Type:     userpb.UserType_USER_TYPE_PRIMARY,
							},
						},
					},
					Owner: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Creator: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Ctime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
					Mtime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
				},
				State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/GetReceivedShare {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// UpdateReceivedShare(ctx context.Context, ref *collaboration.ShareReference, f *collaboration.UpdateReceivedShareRequest_UpdateField) (*collaboration.ReceivedShare, error)
	Describe("UpdateReceivedShare", func() {
		It("calls the UpdateReceivedShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShare, err := am.UpdateReceivedShare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			},
				&collaboration.UpdateReceivedShareRequest_UpdateField{
					Field: &collaboration.UpdateReceivedShareRequest_UpdateField_DisplayName{
						DisplayName: "some new name for this received share",
					},
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(*receivedShare).To(Equal(collaboration.ReceivedShare{
				Share: &collaboration.Share{
					Id:         &collaboration.ShareId{},
					ResourceId: &provider.ResourceId{},
					Permissions: &collaboration.SharePermissions{
						Permissions: &provider.ResourcePermissions{
							AddGrant:             true,
							CreateContainer:      true,
							Delete:               true,
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
							ListGrants:           true,
							ListContainer:        true,
							ListFileVersions:     true,
							ListRecycle:          true,
							Move:                 true,
							RemoveGrant:          true,
							PurgeRecycle:         true,
							RestoreFileVersion:   true,
							RestoreRecycleItem:   true,
							Stat:                 true,
							UpdateGrant:          true,
							DenyGrant:            true,
						},
					},
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: &userpb.UserId{
								Idp:      "0.0.0.0:19000",
								OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
								Type:     userpb.UserType_USER_TYPE_PRIMARY,
							},
						},
					},
					Owner: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Creator: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Ctime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
					Mtime: &types.Timestamp{
						Seconds:              1234567890,
						Nanos:                0,
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     nil,
						XXX_sizecache:        0,
					},
				},
				State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/share/UpdateReceivedShare {"ref":{"Spec":{"Id":{"opaque_id":"some-share-id"}}},"f":{"Field":{"DisplayName":"some new name for this received share"}}}`)
		})
	})

})
