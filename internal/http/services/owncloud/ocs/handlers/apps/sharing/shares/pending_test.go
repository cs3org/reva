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
	"net/http/httptest"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The ocs API", func() {
	var (
		h      *shares.Handler
		client *mocks.GatewayAPIClient

		alice = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		}

		ctx = ctxpkg.ContextSetUser(context.Background(), alice)
	)

	BeforeEach(func() {
		h = &shares.Handler{}
		client = &mocks.GatewayAPIClient{}

		c := &config.Config{}
		c.Init()
		h.InitWithGetter(c, func() (gateway.GatewayAPIClient, error) {
			return client, nil
		})
	})

	Describe("AcceptReceivedShare", func() {
		var (
			resID = &provider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "share1",
			}
			otherResID = &provider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "share3",
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
					Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				},
				ResourceId: resID,
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						Stat:          true,
						ListContainer: true,
					},
				},
			}
			share3 = &collaboration.Share{
				Id: &collaboration.ShareId{OpaqueId: "4"},
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				},
				ResourceId: otherResID,
				Permissions: &collaboration.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						Stat:          true,
						ListContainer: true,
					},
				},
			}
		)

		BeforeEach(func() {
			client.On("GetReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.GetReceivedShareRequest) bool {
				return req.Ref.GetId().GetOpaqueId() == "1"
			})).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: share,
				},
			}, nil)
			client.On("GetReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.GetReceivedShareRequest) bool {
				return req.Ref.GetId().GetOpaqueId() == "2"
			})).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: share2,
				},
			}, nil)
			client.On("GetReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.GetReceivedShareRequest) bool {
				return req.Ref.GetId().GetOpaqueId() == "3"
			})).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: share3,
				},
			}, nil)

			client.On("Stat", mock.Anything, mock.MatchedBy(func(req *provider.StatRequest) bool {
				return req.GetRef().ResourceId.OpaqueId == resID.OpaqueId
			})).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Name:  "share1",
					Id:    resID,
					Owner: alice.Id,
					PermissionSet: &provider.ResourcePermissions{
						Stat: true,
					},
					Size: 10,
				},
			}, nil)

			client.On("Stat", mock.Anything, mock.MatchedBy(func(req *provider.StatRequest) bool {
				return req.GetRef().ResourceId.OpaqueId == otherResID.OpaqueId
			})).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path:  "/share2",
					Id:    otherResID,
					Owner: alice.Id,
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
						Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path:  "/share1",
						Id:    resID,
						Owner: alice.Id,
						Size:  1,
					},
				},
			}, nil)

			client.On("GetUser", mock.Anything, mock.Anything).Return(&userpb.GetUserResponse{
				Status: status.NewOK(context.Background()),
				User:   alice,
			}, nil)
		})

		Context("with one pending share", func() {
			BeforeEach(func() {
				client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share,
							MountPoint: &provider.Reference{Path: "share1"},
						},
					},
				}, nil)
			})

			It("accepts shares", func() {
				client.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
					return req.Share.Share.Id.OpaqueId == "1"
				})).Return(&collaboration.UpdateReceivedShareResponse{
					Status: status.NewOK(context.Background()),
					Share: &collaboration.ReceivedShare{
						State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share:      share,
						MountPoint: &provider.Reference{Path: "share1"},
					},
				}, nil)

				req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares/pending/1", nil)
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("shareid", "1")
				req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

				w := httptest.NewRecorder()
				h.AcceptReceivedShare(w, req)
				Expect(w.Result().StatusCode).To(Equal(200))

				client.AssertNumberOfCalls(GinkgoT(), "UpdateReceivedShare", 1)
			})
		})

		Context("with two pending shares for the same resource", func() {
			BeforeEach(func() {
				client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share,
							MountPoint: &provider.Reference{Path: "share1"},
						},
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share2,
							MountPoint: &provider.Reference{Path: "share2"},
						},
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share3,
							MountPoint: &provider.Reference{Path: "share3"},
						},
					},
				}, nil)
			})

			It("accepts both pending shares", func() {
				client.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
					return req.Share.Share.Id.OpaqueId == "1" && req.Share.MountPoint.Path == "share1"
				})).Return(&collaboration.UpdateReceivedShareResponse{
					Status: status.NewOK(context.Background()),
					Share: &collaboration.ReceivedShare{
						State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share:      share,
						MountPoint: &provider.Reference{Path: "share1"},
					},
				}, nil)

				client.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
					return req.Share.Share.Id.OpaqueId == "2" && req.Share.MountPoint.Path == "share1"
				})).Return(&collaboration.UpdateReceivedShareResponse{
					Status: status.NewOK(context.Background()),
					Share: &collaboration.ReceivedShare{
						State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share:      share2,
						MountPoint: &provider.Reference{Path: "share2"},
					},
				}, nil)

				req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares/pending/1", nil)
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("shareid", "1")
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

				w := httptest.NewRecorder()
				h.AcceptReceivedShare(w, req)
				Expect(w.Result().StatusCode).To(Equal(200))

				client.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
				client.AssertNumberOfCalls(GinkgoT(), "UpdateReceivedShare", 2)
			})
		})

		Context("with one accepted and one pending share for the same resource", func() {
			BeforeEach(func() {
				client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{
						{
							State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
							Share:      share,
							MountPoint: &provider.Reference{Path: "existing/mountpoint"},
						},
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share2,
							MountPoint: &provider.Reference{Path: "share2"},
						},
						{
							State:      collaboration.ShareState_SHARE_STATE_PENDING,
							Share:      share3,
							MountPoint: &provider.Reference{Path: "share3"},
						},
					},
				}, nil)
			})

			It("accepts the remaining pending share", func() {
				client.On("UpdateReceivedShare", mock.Anything, mock.MatchedBy(func(req *collaboration.UpdateReceivedShareRequest) bool {
					return req.Share.Share.Id.OpaqueId == "2" && req.Share.MountPoint.Path == "existing/mountpoint"
				})).Return(&collaboration.UpdateReceivedShareResponse{
					Status: status.NewOK(context.Background()),
					Share: &collaboration.ReceivedShare{
						State:      collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share:      share2,
						MountPoint: &provider.Reference{Path: "share2"},
					},
				}, nil)

				req := httptest.NewRequest("POST", "/apps/files_sharing/api/v1/shares/pending/2", nil)
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("shareid", "2")
				req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

				w := httptest.NewRecorder()
				h.AcceptReceivedShare(w, req)
				Expect(w.Result().StatusCode).To(Equal(200))

				client.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
				client.AssertNumberOfCalls(GinkgoT(), "UpdateReceivedShare", 1)
			})
		})
	})
})
