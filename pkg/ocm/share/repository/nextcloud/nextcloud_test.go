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

package nextcloud_test

import (
	"context"
	"os"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	masked_share "github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/nextcloud"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"
)

func setUpNextcloudServer() (*nextcloud.Manager, *[]string, func()) {
	var conf *nextcloud.ShareManagerConfig

	ncHost := os.Getenv("NEXTCLOUD")
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
	}
	conf = &nextcloud.ShareManagerConfig{
		EndPoint: ncHost + "/apps/sciencemesh/",
		MockHTTP: false,
	}
	nc, _ := nextcloud.NewShareManager(conf)
	return nc, nil, func() {}
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

		options = map[string]interface{}{
			"endpoint":  "http://mock.com/",
			"mock_http": true,
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
			_, err := nextcloud.New(context.Background(), options)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// Share(ctx context.Context, md *provider.ResourceInfo, g *ocm.ShareGrant) (*ocm.Share, error)
	// FIXME: this triggers a call to the Send function from pkg/ocm/share/sender/sender.go
	// which makes an outgoing network call. For the Nextcloud share manager itself we set the
	// `mock_http` config variable, but not sure how to support the network call made by that
	// other package.
	// Describe("Share", func() {
	// 	It("calls the addSentShare endpoint", func() {
	// 		am, called, teardown := setUpNextcloudServer()
	// 		defer teardown()
	// 		var md = &provider.ResourceId{
	// 			StorageId: "",
	// 			OpaqueId:  "fileid-/some/path",
	// 		}
	// 		var g = &ocm.ShareGrant{
	// 			Grantee: &provider.Grantee{
	// 				Id: &provider.Grantee_UserId{
	// 					UserId: &userpb.UserId{
	// 						Idp:      "0.0.0.0:19000",
	// 						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 						Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 					},
	// 				},
	// 			},
	// 			Permissions: &ocm.SharePermissions{
	// 				Permissions: &provider.ResourcePermissions{
	// 					AddGrant:             false,
	// 					CreateContainer:      false,
	// 					Delete:               false,
	// 					GetPath:              true,
	// 					GetQuota:             false,
	// 					InitiateFileDownload: false,
	// 					InitiateFileUpload:   false,
	// 					ListGrants:           false,
	// 					ListContainer:        false,
	// 					ListFileVersions:     false,
	// 					ListRecycle:          false,
	// 					Move:                 false,
	// 					RemoveGrant:          false,
	// 					PurgeRecycle:         false,
	// 					RestoreFileVersion:   false,
	// 					RestoreRecycleItem:   false,
	// 					Stat:                 false,
	// 					UpdateGrant:          false,
	// 					DenyGrant:            false,
	// 				},
	// 			},
	// 		}
	// 		var name = "Some Name"
	// 		var pi = &ocmprovider.ProviderInfo{}
	// 		var pm = "some-permissions-string?"
	// 		var owner = &userpb.UserId{
	// 			Idp:      "0.0.0.0:19000",
	// 			OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 			Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 		}
	// 		var token = "some-token"
	// 		var st = ocm.Share_SHARE_TYPE_REGULAR
	// 		share, err := am.Share(ctx, md, g, name, pi, pm, owner, token, st)

	// 		Expect(err).ToNot(HaveOccurred())
	// 		Expect(*share).To(Equal(ocm.Share{
	// 			Id:         &ocm.ShareId{},
	// 			ResourceId: &provider.ResourceId{},
	// 			Permissions: &ocm.SharePermissions{
	// 				Permissions: &provider.ResourcePermissions{
	// 					AddGrant:             true,
	// 					CreateContainer:      true,
	// 					Delete:               true,
	// 					GetPath:              true,
	// 					GetQuota:             true,
	// 					InitiateFileDownload: true,
	// 					InitiateFileUpload:   true,
	// 					ListGrants:           true,
	// 					ListContainer:        true,
	// 					ListFileVersions:     true,
	// 					ListRecycle:          true,
	// 					Move:                 true,
	// 					RemoveGrant:          true,
	// 					PurgeRecycle:         true,
	// 					RestoreFileVersion:   true,
	// 					RestoreRecycleItem:   true,
	// 					Stat:                 true,
	// 					UpdateGrant:          true,
	// 					DenyGrant:            true,
	// 				},
	// 			},
	// 			Grantee: &provider.Grantee{
	// 				Id: &provider.Grantee_UserId{
	// 					UserId: &userpb.UserId{
	// 						Idp:      "0.0.0.0:19000",
	// 						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 						Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 					},
	// 				},
	// 			},
	// 			Owner: &userpb.UserId{
	// 				Idp:      "0.0.0.0:19000",
	// 				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 				Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 			},
	// 			Creator: &userpb.UserId{
	// 				Idp:      "0.0.0.0:19000",
	// 				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 				Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 			},
	// 			Ctime: &types.Timestamp{
	// 				Seconds:              1234567890,
	// 				Nanos:                0,
	// 				XXX_NoUnkeyedLiteral: struct{}{},
	// 				XXX_unrecognized:     nil,
	// 				XXX_sizecache:        0,
	// 			},
	// 			Mtime: &types.Timestamp{
	// 				Seconds:              1234567890,
	// 				Nanos:                0,
	// 				XXX_NoUnkeyedLiteral: struct{}{},
	// 				XXX_unrecognized:     nil,
	// 				XXX_sizecache:        0,
	// 			},
	// 		}))
	// 		checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/addReceivedShare {"md":{"opaque_id":"fileid-/some/path"},"g":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"permissions":{"get_path":true}}},"provider_domain":"cern.ch","resource_type":"file","provider_id":2,"owner_opaque_id":"einstein","owner_display_name":"Albert Einstein","protocol":{"name":"webdav","options":{"sharedSecret":"secret","permissions":"webdav-property"}}}`)
	// 	})
	// })

	// GetShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.Share, error)
	Describe("GetShare", func() {
		It("calls the GetShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			share, err := am.GetShare(ctx, user, &ocm.ShareReference{
				Spec: &ocm.ShareReference_Id{
					Id: &ocm.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*share).To(Equal(ocm.Share{
				Id:         &ocm.ShareId{},
				ResourceId: &provider.ResourceId{},
				Name:       "",
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				AccessMethods: []*ocm.AccessMethod{
					masked_share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					// masked_share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					// masked_share.NewTransferAccessMethod(),
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
				ShareType: ocm.ShareType_SHARE_TYPE_USER,
				Token:     "some-token",
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/GetSentShareByToken {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// Unshare(ctx context.Context, ref *ocm.ShareReference) error
	Describe("Unshare", func() {
		It("calls the Unshare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			err := am.DeleteShare(ctx, user, &ocm.ShareReference{
				Spec: &ocm.ShareReference_Id{
					Id: &ocm.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/Unshare {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// UpdateShare(ctx context.Context, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error)
	// Describe("UpdateShare", func() {
	// 	It("calls the UpdateShare endpoint", func() {
	// 		am, called, teardown := setUpNextcloudServer()
	// 		defer teardown()

	// 		share, err := am.UpdateShare(ctx, user, &ocm.ShareReference{
	// 			Spec: &ocm.ShareReference_Id{
	// 				Id: &ocm.ShareId{
	// 					OpaqueId: "some-share-id",
	// 				},
	// 			},
	// 		})
	// 		Expect(err).ToNot(HaveOccurred())
	// 		Expect(*share).To(Equal(ocm.Share{
	// 			Id: &ocm.ShareId{},
	// 			Grantee: &provider.Grantee{
	// 				Id: &provider.Grantee_UserId{
	// 					UserId: &userpb.UserId{
	// 						Idp:      "0.0.0.0:19000",
	// 						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 						Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 					},
	// 				},
	// 			},
	// 			Owner: &userpb.UserId{
	// 				Idp:      "0.0.0.0:19000",
	// 				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 				Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 			},
	// 			Creator: &userpb.UserId{
	// 				Idp:      "0.0.0.0:19000",
	// 				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	// 				Type:     userpb.UserType_USER_TYPE_PRIMARY,
	// 			},
	// 			Ctime: &types.Timestamp{
	// 				Seconds:              1234567890,
	// 				Nanos:                0,
	// 				XXX_NoUnkeyedLiteral: struct{}{},
	// 				XXX_unrecognized:     nil,
	// 				XXX_sizecache:        0,
	// 			},
	// 			Mtime: &types.Timestamp{
	// 				Seconds:              1234567890,
	// 				Nanos:                0,
	// 				XXX_NoUnkeyedLiteral: struct{}{},
	// 				XXX_unrecognized:     nil,
	// 				XXX_sizecache:        0,
	// 			},
	// 		}))
	// 		checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/UpdateShare {"ref":{"Spec":{"Id":{"opaque_id":"some-share-id"}}},"p":{"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`)
	// 	})
	// })

	// ListShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error)
	Describe("ListShares", func() {
		It("calls the ListShares endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			shares, err := am.ListShares(ctx, user, []*ocm.ListOCMSharesRequest_Filter{
				{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_CREATOR,
					Term: &ocm.ListOCMSharesRequest_Filter_Creator{
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
			Expect(*shares[0]).To(Equal(ocm.Share{
				Id: &ocm.ShareId{},
				ResourceId: &provider.ResourceId{
					StorageId: "",
					OpaqueId:  "",
					SpaceId:   "",
				},
				Name: "",
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
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
				ShareType: ocm.ShareType_SHARE_TYPE_USER,
				AccessMethods: []*ocm.AccessMethod{
					masked_share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					// masked_share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					// masked_share.NewTransferAccessMethod(),
				},
				Token: "some-token",
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/ListShares [{"type":4,"Term":{"Creator":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}}]`)
		})
	})

	// ListReceivedShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.ReceivedShare, error)
	Describe("ListReceivedShares", func() {
		It("calls the ListReceivedShares endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShares, err := am.ListReceivedShares(ctx, user)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(receivedShares)).To(Equal(1))
			Expect(*receivedShares[0]).To(Equal(ocm.ReceivedShare{
				Id:            &ocm.ShareId{},
				Name:          "",
				RemoteShareId: "",
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
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
				ShareType: ocm.ShareType_SHARE_TYPE_USER,
				State:     ocm.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/ListReceivedShares `)
		})
	})

	// GetReceivedShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error)
	Describe("GetReceivedShare", func() {
		It("calls the GetReceivedShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShare, err := am.GetReceivedShare(ctx, user, &ocm.ShareReference{
				Spec: &ocm.ShareReference_Id{
					Id: &ocm.ShareId{
						OpaqueId: "some-share-id",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*receivedShare).To(Equal(ocm.ReceivedShare{
				Id:            &ocm.ShareId{},
				Name:          "",
				RemoteShareId: "",
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
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
				ShareType: ocm.ShareType_SHARE_TYPE_USER,
				State:     ocm.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/GetReceivedShare {"Spec":{"Id":{"opaque_id":"some-share-id"}}}`)
		})
	})

	// UpdateReceivedShare(ctx context.Context, receivedShare *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error)
	Describe("UpdateReceivedShare", func() {
		It("calls the UpdateReceivedShare endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

			receivedShare, err := am.UpdateReceivedShare(ctx, user,
				&ocm.ReceivedShare{
					Id:            &ocm.ShareId{},
					Name:          "",
					RemoteShareId: "",
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
					ShareType: ocm.ShareType_SHARE_TYPE_USER,
					State:     ocm.ShareState_SHARE_STATE_ACCEPTED,
				},
				&field_mask.FieldMask{
					Paths: []string{"state"},
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(*receivedShare).To(Equal(ocm.ReceivedShare{
				Id:            &ocm.ShareId{},
				Name:          "",
				RemoteShareId: "",
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						},
					},
				},
				Owner: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				},
				Creator: &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
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
				ShareType: ocm.ShareType_SHARE_TYPE_USER,
				State:     ocm.ShareState_SHARE_STATE_ACCEPTED,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/ocm/UpdateReceivedShare {"received_share":{"id":{},"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"owner":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1},"creator":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1},"ctime":{"seconds":1234567890},"mtime":{"seconds":1234567890},"share_type":1,"state":2},"field_mask":{"paths":["state"]}}`)
		})
	})

})
