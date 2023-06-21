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

package shares_test

import (
	"context"
	"encoding/xml"
	"math/rand"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/config"
	cdata "github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/data"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	cs3mocks "github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The ocs API", func() {
	var (
		h             *shares.Handler
		gatewayClient *cs3mocks.GatewayAPIClient

		user = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "admin",
			},
		}

		ctx = ctxpkg.ContextSetUser(context.Background(), user)
	)
	type (
		share struct {
			ID                string `xml:"id"`
			ShareType         string `xml:"share_type"`
			ShareWithUserType string `xml:"share_with_user_type"`
		}
		data struct {
			Shares []share `xml:"element"`
		}
		response struct {
			Data data `xml:"data"`
		}
	)
	BeforeEach(func() {
		h = &shares.Handler{}
		pool.RemoveSelector("GatewaySelector" + "any")
		gatewayClient = &cs3mocks.GatewayAPIClient{}

		c := &config.Config{}
		c.GatewaySvc = "gatewaysvc"
		c.StatCacheDatabase = strconv.FormatInt(rand.Int63(), 10) // Use a fresh database for each test
		c.Init()
		h.InitWithGetter(c, func() (gateway.GatewayAPIClient, error) {
			return gatewayClient, nil
		})
	})

	Describe("CreateShare", func() {
		BeforeEach(func() {
			gatewayClient.On("GetUserByClaim", mock.Anything, mock.Anything).Return(&userpb.GetUserByClaimResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			gatewayClient.On("Authenticate", mock.Anything, mock.Anything).Return(&gateway.AuthenticateResponse{
				Status: status.NewOK(context.Background()),
			}, nil)

			gatewayClient.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{
				Status: status.NewOK(context.Background()),
			}, nil)
		})

		Context("when sharing the personal space root", func() {
			BeforeEach(func() {
				gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
					Status: status.NewOK(context.Background()),
					Info: &provider.ResourceInfo{
						Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path: "/",
						Id: &provider.ResourceId{
							StorageId: "storageid",
							SpaceId:   "spaceid",
							OpaqueId:  "spaceid",
						},
						Owner: user.Id,
						PermissionSet: &provider.ResourcePermissions{
							Stat:        true,
							AddGrant:    true,
							UpdateGrant: true,
							RemoveGrant: true,
						},
						Size: 10,
						Space: &provider.StorageSpace{
							SpaceType: "personal",
						},
					},
				}, nil)
			})

			It("does not create a user share", func() {
				form := url.Values{}
				form.Add("shareType", "0")
				form.Add("path", "/")
				form.Add("spaceRef", "storageid!spaceid:spaceid")
				form.Add("permissions", "1")
				form.Add("role", "viewer")
				form.Add("shareWith", "admin")
				req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req = req.WithContext(ctx)

				w := httptest.NewRecorder()
				h.CreateShare(w, req)
				Expect(w.Result().StatusCode).To(Equal(400))
				gatewayClient.AssertNumberOfCalls(GinkgoT(), "CreateShare", 0)
			})
		})

		Context("when sharing a project space root via link", func() {
			BeforeEach(func() {
				var (
					resID = &provider.ResourceId{
						StorageId: "share1-storageid",
						OpaqueId:  "share1",
					}
				)

				gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
					Status: status.NewOK(context.Background()),
					Info: &provider.ResourceInfo{
						Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path:  "/",
						Id:    resID,
						Owner: user.Id,
						PermissionSet: &provider.ResourcePermissions{
							Stat:                 true,
							ListContainer:        true,
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							AddGrant:             true,
							ListGrants:           true,
							ListRecycle:          true,
							UpdateGrant:          true,
							RemoveGrant:          true,
						},
						Size: 10,
						Space: &provider.StorageSpace{
							SpaceType: "project",
						},
					},
				}, nil)

				gatewayClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&permissions.CheckPermissionResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				}, nil)

				gatewayClient.On("CreatePublicShare", mock.Anything, mock.Anything).Return(&link.CreatePublicShareResponse{
					Status: status.NewOK(context.Background()),
					Share: &link.PublicShare{
						Token:   "foo",
						Creator: user.Id,
						Owner:   user.Id,
					},
				}, nil)
			})

			It("creates a link share", func() {
				form := url.Values{}
				form.Add("shareType", "3")
				form.Add("path", "/")
				form.Add("space", "storageid!spaceid")
				form.Add("permissions", "1")
				req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req = req.WithContext(ctx)

				w := httptest.NewRecorder()
				h.CreateShare(w, req)
				Expect(w.Result().StatusCode).To(Equal(200))
			})
		})

		Context("when sharing a resource", func() {
			var (
				resID = &provider.ResourceId{
					StorageId: "share1-storageid",
					OpaqueId:  "share1",
				}
				share = &collaboration.Share{
					Id: &collaboration.ShareId{OpaqueId: "1"},
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
					},
					ResourceId: resID,
					Permissions: &collaboration.SharePermissions{
						Permissions: &provider.ResourcePermissions{
							Stat:          true,
							ListContainer: true,
						},
					},
				}
				share2 = &collaboration.Share{
					Id: &collaboration.ShareId{OpaqueId: "2"},
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
					},
					ResourceId: resID,
					Permissions: &collaboration.SharePermissions{
						Permissions: &provider.ResourcePermissions{
							Stat:          true,
							ListContainer: true,
						},
					},
				}
			)

			BeforeEach(func() {
				gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
					Status: status.NewOK(context.Background()),
					Info: &provider.ResourceInfo{
						Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path:  "/newshare",
						Id:    resID,
						Owner: user.Id,
						PermissionSet: &provider.ResourcePermissions{
							Stat:        true,
							AddGrant:    true,
							UpdateGrant: true,
							RemoveGrant: true,
						},
						Size: 10,
					},
				}, nil)

				gatewayClient.On("GetShare", mock.Anything, mock.Anything).Return(&collaboration.GetShareResponse{
					Status: status.NewOK(context.Background()),
					Share:  share,
				}, nil)
			})

			Context("when there are no existing shares to the resource yet", func() {
				BeforeEach(func() {
					gatewayClient.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
						Status: status.NewOK(context.Background()),
						Shares: []*collaboration.ReceivedShare{
							{
								State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
								Share:      share,
								MountPoint: &provider.Reference{Path: ""},
							},
						},
					}, nil)
				})

				It("creates a new share", func() {
					gatewayClient.On("CreateShare", mock.Anything, mock.Anything).Return(&collaboration.CreateShareResponse{
						Status: status.NewOK(context.Background()),
						Share:  share,
					}, nil)

					form := url.Values{}
					form.Add("shareType", "0")
					form.Add("path", "/newshare")
					form.Add("name", "newshare")
					form.Add("permissions", "16")
					form.Add("shareWith", "admin")
					req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					req = req.WithContext(ctx)

					w := httptest.NewRecorder()
					h.CreateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(200))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "CreateShare", 1)
				})
			})

			Context("when a share to the same resource already exists", func() {
				BeforeEach(func() {
					gatewayClient.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
						Status: status.NewOK(context.Background()),
						Shares: []*collaboration.ReceivedShare{
							{
								State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
								Share:      share,
								MountPoint: &provider.Reference{Path: "some-mountpoint"},
							},
							{
								State: collaboration.ShareState_SHARE_STATE_PENDING,
								Share: share2,
							},
						},
					}, nil)
				})

				It("auto-accepts the share and applies the mountpoint", func() {
					gatewayClient.On("CreateShare", mock.Anything, mock.Anything).Return(&collaboration.CreateShareResponse{
						Status: status.NewOK(context.Background()),
						Share:  share2,
					}, nil)
					gatewayClient.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
						return req.Share.Share.Id.OpaqueId == "2" && req.Share.MountPoint.Path == "some-mountpoint" && req.Share.State == collaboration.ShareState_SHARE_STATE_ACCEPTED
					})).Return(&collaboration.UpdateReceivedShareResponse{
						Status: status.NewOK(context.Background()),
						Share: &collaboration.ReceivedShare{
							State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
							Share:      share2,
							MountPoint: &provider.Reference{Path: "share2"},
						},
					}, nil)

					form := url.Values{}
					form.Add("shareType", "0")
					form.Add("path", "/newshare")
					form.Add("name", "newshare")
					form.Add("permissions", "16")
					form.Add("shareWith", "admin")
					req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					req = req.WithContext(ctx)

					w := httptest.NewRecorder()
					h.CreateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(200))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "CreateShare", 1)
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdateReceivedShare", 1)
				})
			})
		})

		It("does not allow adding space members to a personal space", func() {
			gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path: "/",
					Id: &provider.ResourceId{
						StorageId: "storageid",
						SpaceId:   "spaceid",
						OpaqueId:  "spaceid",
					},
					Owner: user.Id,
					PermissionSet: &provider.ResourcePermissions{
						Stat:                 true,
						GetPath:              true,
						GetQuota:             true,
						InitiateFileDownload: true,
						ListRecycle:          true,
						ListContainer:        true,
						AddGrant:             true,
						UpdateGrant:          true,
						RemoveGrant:          true,
					},
					Size: 10,
					Space: &provider.StorageSpace{
						SpaceType: "personal",
					},
				},
			}, nil)

			form := url.Values{}
			form.Add("shareType", "7")
			form.Add("path", "/")
			form.Add("spaceRef", "storageid!spaceid")
			form.Add("permissions", "1")
			form.Add("role", "viewer")
			form.Add("shareWith", "admin")
			req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			h.CreateShare(w, req)
			Expect(w.Result().StatusCode).To(Equal(400))
			gatewayClient.AssertNumberOfCalls(GinkgoT(), "CreateShare", 0)
		})
	})

	Describe("UpdatePublicShare", func() {
		BeforeEach(func() {
			gatewayClient.On("GetUserByClaim", mock.Anything, mock.Anything).Return(&userpb.GetUserByClaimResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			gatewayClient.On("Authenticate", mock.Anything, mock.Anything).Return(&gateway.AuthenticateResponse{
				Status: status.NewOK(context.Background()),
			}, nil)
		})

		Context("Password Enforced when update a share", func() {
			var (
				resID = &provider.ResourceId{
					StorageId: "share1-storageid",
					OpaqueId:  "share1",
				}
				share2 = &link.PublicShare{
					Id:                &link.PublicShareId{OpaqueId: "2"},
					ResourceId:        resID,
					Owner:             user.Id,
					Quicklink:         true,
					PasswordProtected: false,
				}

				statResponse = &provider.StatResponse{
					Status: status.NewOK(context.Background()),
					Info: &provider.ResourceInfo{
						Type:  provider.ResourceType_RESOURCE_TYPE_FILE,
						Path:  "/2",
						Id:    resID,
						Owner: user.Id,
						PermissionSet: &provider.ResourcePermissions{
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
							ListContainer:        true,
							ListRecycle:          true,
							RestoreRecycleItem:   true,
							Stat:                 true,
						},
						Size: 10,
					},
				}
			)

			BeforeEach(func() {
				h = &shares.Handler{}
				pool.RemoveSelector("GatewaySelector" + "any")
				gatewayClient = &cs3mocks.GatewayAPIClient{}

				c := &config.Config{}
				c.GatewaySvc = "gatewaysvc"
				c.StatCacheDatabase = strconv.FormatInt(rand.Int63(), 10) // Use a fresh database for each test
				// this is equivalent of the ocis OCIS_SHARING_PUBLIC_WRITEABLE_SHARE_MUST_HAVE_PASSWORD=true
				c.Capabilities = cdata.CapabilitiesData{
					Capabilities: &cdata.Capabilities{FilesSharing: &cdata.CapabilitiesFilesSharing{Public: &cdata.CapabilitiesFilesSharingPublic{
						Password: &cdata.CapabilitiesFilesSharingPublicPassword{
							EnforcedFor: &cdata.CapabilitiesFilesSharingPublicPasswordEnforcedFor{
								ReadOnly:        false,
								ReadWrite:       true,
								ReadWriteDelete: true,
								UploadOnly:      true,
							},
						}}}},
				}
				c.Init()
				h.InitWithGetter(c, func() (gateway.GatewayAPIClient, error) {
					return gatewayClient, nil
				})
				gatewayClient.On("GetPublicShare", mock.Anything, mock.Anything).Return(&link.GetPublicShareResponse{
					Status: status.NewOK(context.Background()),
					Share:  share2,
				}, nil)

				gatewayClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&permissions.CheckPermissionResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				}, nil)

				gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(statResponse, nil)

				gatewayClient.On("UpdatePublicShare", mock.Anything, mock.Anything).Return(&link.UpdatePublicShareResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_OK},
					Share:  share2,
				}, nil)

				gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
					Status: status.NewOK(context.Background()),
					User:   user,
				}, nil)
			})

			Context("when change the permission to 3", func() {

				It("the password exists. update succeed", func() {
					form := url.Values{}
					form.Add("permissions", "3")
					form.Add("password", "passwass")
					req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("shareid", "2")
					req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

					w := httptest.NewRecorder()
					h.UpdateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(200))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 2)
				})

				It("the password doesn't exist. update failed", func() {
					form := url.Values{}
					form.Add("permissions", "3")
					req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("shareid", "2")
					req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

					w := httptest.NewRecorder()
					h.UpdateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(400))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 0)
				})

				It("permissions=1, the password doesn't exist. update succeed", func() {
					form := url.Values{}
					form.Add("permissions", "1")
					req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("shareid", "2")
					req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

					w := httptest.NewRecorder()
					h.UpdateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(200))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 1)
				})
			})
		})

		Context("Password Enforced when update a share", func() {
			var (
				resID = &provider.ResourceId{
					StorageId: "share1-storageid",
					OpaqueId:  "share1",
				}
				share2 = &link.PublicShare{
					Id:                &link.PublicShareId{OpaqueId: "3"},
					ResourceId:        resID,
					Owner:             user.Id,
					Quicklink:         true,
					PasswordProtected: false,
				}

				statResponse = &provider.StatResponse{
					Status: status.NewOK(context.Background()),
					Info: &provider.ResourceInfo{
						Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path:  "/3",
						Id:    resID,
						Owner: user.Id,
						PermissionSet: &provider.ResourcePermissions{
							AddGrant:             true,
							CreateContainer:      true,
							Delete:               true,
							GetPath:              true,
							GetQuota:             true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
							ListGrants:           true,
							ListContainer:        true,
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
				}
			)

			BeforeEach(func() {
				h = &shares.Handler{}
				pool.RemoveSelector("GatewaySelector" + "any")
				gatewayClient = &cs3mocks.GatewayAPIClient{}

				c := &config.Config{}
				c.GatewaySvc = "gatewaysvc"
				c.StatCacheDatabase = strconv.FormatInt(rand.Int63(), 10) // Use a fresh database for each test
				// this is equivalent of the ocis OCIS_SHARING_PUBLIC_WRITEABLE_SHARE_MUST_HAVE_PASSWORD=true
				c.Capabilities = cdata.CapabilitiesData{
					Capabilities: &cdata.Capabilities{FilesSharing: &cdata.CapabilitiesFilesSharing{Public: &cdata.CapabilitiesFilesSharingPublic{
						Password: &cdata.CapabilitiesFilesSharingPublicPassword{
							EnforcedFor: &cdata.CapabilitiesFilesSharingPublicPasswordEnforcedFor{
								ReadOnly:        false,
								ReadWrite:       true,
								ReadWriteDelete: true,
								UploadOnly:      true,
							},
						}}}},
				}
				c.Init()
				h.InitWithGetter(c, func() (gateway.GatewayAPIClient, error) {
					return gatewayClient, nil
				})
				gatewayClient.On("GetPublicShare", mock.Anything, mock.Anything).Return(&link.GetPublicShareResponse{
					Status: status.NewOK(context.Background()),
					Share:  share2,
				}, nil)

				gatewayClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&permissions.CheckPermissionResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				}, nil)

				gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(statResponse, nil)

				gatewayClient.On("UpdatePublicShare", mock.Anything, mock.Anything).Return(&link.UpdatePublicShareResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_OK},
					Share:  share2,
				}, nil)

				gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
					Status: status.NewOK(context.Background()),
					User:   user,
				}, nil)
			})

			Context("when change the permission", func() {
				for _, perm := range []string{"4", "5", "15"} {
					perm := perm
					It("the password exists. update succeed", func() {
						form := url.Values{}
						form.Add("permissions", perm)
						form.Add("password", "passwass")
						req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
						req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
						rctx := chi.NewRouteContext()
						rctx.URLParams.Add("shareid", "3")
						req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

						w := httptest.NewRecorder()
						h.UpdateShare(w, req)
						Expect(w.Result().StatusCode).To(Equal(200))
						gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 2)
					})
				}

				It("the password doesn't exist. update failed", func() {
					form := url.Values{}
					form.Add("permissions", "3")
					req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("shareid", "3")
					req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

					w := httptest.NewRecorder()
					h.UpdateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(400))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 0)
				})

				It("permissions=1, the password doesn't exist. update succeed", func() {
					form := url.Values{}
					form.Add("permissions", "1")
					req := httptest.NewRequest("PUT", "/ocs/v1.php/apps/files_sharing/api/v1/shares", strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("shareid", "3")
					req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

					w := httptest.NewRecorder()
					h.UpdateShare(w, req)
					Expect(w.Result().StatusCode).To(Equal(200))
					gatewayClient.AssertNumberOfCalls(GinkgoT(), "UpdatePublicShare", 1)
				})
			})
		})
	})

	Describe("ListShares", func() {
		BeforeEach(func() {
			resID := &provider.ResourceId{
				StorageId: "share1-storageid",
				SpaceId:   "space-1",
				OpaqueId:  "share1",
			}
			gatewayClient.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					{
						State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaboration.Share{
							Id: &collaboration.ShareId{OpaqueId: "10"},
							Grantee: &provider.Grantee{
								Type: provider.GranteeType_GRANTEE_TYPE_USER,
							},
							Creator:    user.Id,
							ResourceId: resID,
							Permissions: &collaboration.SharePermissions{
								Permissions: &provider.ResourcePermissions{
									Stat:          true,
									ListContainer: true,
								},
							},
						},
						MountPoint: &provider.Reference{Path: "share1"},
					},
				},
			}, nil)

			gatewayClient.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.Share{
					{
						Id: &collaboration.ShareId{OpaqueId: "11"},
						Grantee: &provider.Grantee{
							Type: provider.GranteeType_GRANTEE_TYPE_USER,
						},
						Creator:    user.Id,
						ResourceId: resID,
						Permissions: &collaboration.SharePermissions{
							Permissions: &provider.ResourcePermissions{
								Stat:          true,
								ListContainer: true,
							},
						},
					},
				},
			}, nil)

			gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path:  "/share1",
					Id:    resID,
					Owner: user.Id,
					PermissionSet: &provider.ResourcePermissions{
						Stat: true,
					},
					Size: 10,
				},
			}, nil)

			gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
		})

		It("lists accepted shares", func() {
			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?shared_with_me=1", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(1))
			s := res.Data.Shares[0]
			Expect(s.ID).To(Equal("10"))
		})
		It("lists shares as creator", func() {
			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?reshares=true", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(1))
			s := res.Data.Shares[0]
			Expect(s.ID).To(Equal("11"))
		})
		It("lists shares with another user", func() {
			user0 := &userpb.User{
				Id: &userpb.UserId{
					OpaqueId: helpers.User0ID,
				},
			}

			ctx0 := ctxpkg.ContextSetUser(context.Background(), user0)

			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?reshares=true", nil).WithContext(ctx0)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(0))
		})
	})
	Describe("ListShares as Space Member", func() {
		BeforeEach(func() {
			resID := &provider.ResourceId{
				StorageId: "share1-storageid",
				SpaceId:   "space-1",
				OpaqueId:  "share1",
			}
			gatewayClient.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.Share{
					{
						Id: &collaboration.ShareId{OpaqueId: "11"},
						Grantee: &provider.Grantee{
							Type: provider.GranteeType_GRANTEE_TYPE_USER,
						},
						Creator:    user.Id,
						ResourceId: resID,
						Permissions: &collaboration.SharePermissions{
							Permissions: &provider.ResourcePermissions{
								Stat:          true,
								ListContainer: true,
							},
						},
					},
					{
						Id: &collaboration.ShareId{OpaqueId: "12"},
						Grantee: &provider.Grantee{
							Type: provider.GranteeType_GRANTEE_TYPE_USER,
						},
						Creator: &userpb.UserId{
							OpaqueId: helpers.User1ID,
						},
						ResourceId: resID,
						Permissions: &collaboration.SharePermissions{
							Permissions: &provider.ResourcePermissions{
								Stat:          true,
								ListContainer: true,
							},
						},
					},
				},
			}, nil)

			gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path:  "/share1",
					Id:    resID,
					Owner: user.Id,
					PermissionSet: &provider.ResourcePermissions{
						Stat:       true,
						ListGrants: true,
					},
					Size: 10,
				},
			}, nil)

			gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
		})
		It("lists shares inside a space from another user", func() {
			user0 := &userpb.User{
				Id: &userpb.UserId{
					OpaqueId: helpers.User0ID,
				},
			}
			ctx0 := ctxpkg.ContextSetUser(context.Background(), user0)
			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?reshares=true", nil).WithContext(ctx0)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(2))
			s1 := res.Data.Shares[0]
			s2 := res.Data.Shares[1]
			Expect(s1.ID).To(Equal("11"))
			Expect(s1.ShareType).To(Equal("0"))
			Expect(s1.ShareWithUserType).To(Equal("0"))
			Expect(s2.ID).To(Equal("12"))
			Expect(s2.ShareType).To(Equal("0"))
			Expect(s2.ShareWithUserType).To(Equal("0"))
		})
	})
	Describe("List Guest Shares", func() {
		BeforeEach(func() {
			resID := &provider.ResourceId{
				StorageId: "share1-storageid",
				SpaceId:   "space-1",
				OpaqueId:  "share1",
			}
			userGuest := &userpb.User{
				Id: &userpb.UserId{
					OpaqueId: helpers.User0ID,
					Type:     userpb.UserType_USER_TYPE_GUEST,
				},
			}
			gatewayClient.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					{
						State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaboration.Share{
							Id: &collaboration.ShareId{OpaqueId: "10"},
							Grantee: &provider.Grantee{
								Type: provider.GranteeType_GRANTEE_TYPE_USER,
								Id: &provider.Grantee_UserId{
									UserId: userGuest.Id,
								},
							},
							Creator:    user.Id,
							ResourceId: resID,
							Permissions: &collaboration.SharePermissions{
								Permissions: &provider.ResourcePermissions{
									Stat:          true,
									ListContainer: true,
								},
							},
						},
						MountPoint: &provider.Reference{Path: "share1"},
					},
				},
			}, nil)

			gatewayClient.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.Share{
					{
						Id: &collaboration.ShareId{OpaqueId: "10"},
						Grantee: &provider.Grantee{
							Type: provider.GranteeType_GRANTEE_TYPE_USER,
							Id: &provider.Grantee_UserId{
								UserId: userGuest.Id,
							},
						},
						Creator:    user.Id,
						ResourceId: resID,
						Permissions: &collaboration.SharePermissions{
							Permissions: &provider.ResourcePermissions{
								Stat:          true,
								ListContainer: true,
							},
						},
					},
				},
			}, nil)

			gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path:  "/share1",
					Id:    resID,
					Owner: user.Id,
					PermissionSet: &provider.ResourcePermissions{
						Stat: true,
					},
					Size: 10,
				},
			}, nil)

			gatewayClient.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
		})
		It("lists guest shares as creator", func() {
			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?reshares=true", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(1))
			s := res.Data.Shares[0]
			Expect(s.ID).To(Equal("10"))
			Expect(s.ShareWithUserType).To(Equal("1"))
			Expect(s.ShareType).To(Equal("0"))
		})
	})
})
