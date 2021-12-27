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
	"net/http/httptest"
	"net/url"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares/mocks"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The ocs API", func() {
	var (
		h      *shares.Handler
		client *mocks.GatewayClient

		user = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "admin",
			},
		}

		ctx = ctxpkg.ContextSetUser(context.Background(), user)
	)

	BeforeEach(func() {
		h = &shares.Handler{}
		client = &mocks.GatewayClient{}

		c := &config.Config{}
		c.Init()
		h.InitWithGetter(c, func() (shares.GatewayClient, error) {
			return client, nil
		})
	})

	Describe("CreateShare", func() {
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
			client.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path: "/newshare",
					Id:   resID,
					PermissionSet: &provider.ResourcePermissions{
						Stat:        true,
						AddGrant:    true,
						UpdateGrant: true,
						RemoveGrant: true,
					},
					Size: 10,
				},
			}, nil)

			client.On("GetUserByClaim", mock.Anything, mock.Anything).Return(&userpb.GetUserByClaimResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			client.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
			client.On("Authenticate", mock.Anything, mock.Anything).Return(&gateway.AuthenticateResponse{
				Status: status.NewOK(context.Background()),
			}, nil)

			client.On("GetShare", mock.Anything, mock.Anything).Return(&collaboration.GetShareResponse{
				Status: status.NewOK(context.Background()),
				Share:  share,
			}, nil)
		})

		Context("when there are no existing shares to the resource yet", func() {
			BeforeEach(func() {
				client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
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
				client.On("CreateShare", mock.Anything, mock.Anything).Return(&collaboration.CreateShareResponse{
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
				client.AssertNumberOfCalls(GinkgoT(), "CreateShare", 1)
			})
		})

		Context("when a share to the same resource already exists", func() {
			BeforeEach(func() {
				client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
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
				client.On("CreateShare", mock.Anything, mock.Anything).Return(&collaboration.CreateShareResponse{
					Status: status.NewOK(context.Background()),
					Share:  share2,
				}, nil)
				client.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
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
				client.AssertNumberOfCalls(GinkgoT(), "CreateShare", 1)
				client.AssertNumberOfCalls(GinkgoT(), "UpdateReceivedShare", 1)
			})
		})
	})

	Describe("ListShares", func() {
		BeforeEach(func() {
			resID := &provider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "share1",
			}
			client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					{
						State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaboration.Share{
							Id: &collaboration.ShareId{OpaqueId: "10"},
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
						},
						MountPoint: &provider.Reference{Path: "share1"},
					},
				},
			}, nil)

			client.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
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

			client.On("ListContainer", mock.Anything, mock.Anything).Return(&provider.ListContainerResponse{
				Status: status.NewOK(context.Background()),
				Infos: []*provider.ResourceInfo{
					{
						Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path: "/share1",
						Id:   resID,
						Size: 1,
					},
				},
			}, nil)

			client.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   user,
			}, nil)
		})

		It("lists accepted shares", func() {
			type share struct {
				ID string `xml:"id"`
			}
			type data struct {
				Shares []share `xml:"element"`
			}
			type response struct {
				Data data `xml:"data"`
			}

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
	})
})
