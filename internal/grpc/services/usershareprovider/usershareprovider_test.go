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

package usershareprovider_test

import (
	"context"
	"regexp"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationpb "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/cs3org/reva/v2/internal/grpc/services/usershareprovider"
	"github.com/cs3org/reva/v2/pkg/conversions"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/share"
	_ "github.com/cs3org/reva/v2/pkg/share/manager/loader"
	"github.com/cs3org/reva/v2/pkg/share/manager/registry"
	"github.com/cs3org/reva/v2/pkg/share/mocks"
	cs3mocks "github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
)

var _ = Describe("user share provider service", func() {
	var (
		ctx                      context.Context
		provider                 collaborationpb.CollaborationAPIServer
		manager                  *mocks.Manager
		gatewayClient            *cs3mocks.GatewayAPIClient
		gatewaySelector          pool.Selectable[gateway.GatewayAPIClient]
		checkPermissionResponse  *permissions.CheckPermissionResponse
		statResourceResponse     *providerpb.StatResponse
		cs3permissionsNoAddGrant *providerpb.ResourcePermissions
		getShareResponse         *collaborationpb.Share
	)
	cs3permissionsNoAddGrant = conversions.RoleFromName("manager").CS3ResourcePermissions()
	cs3permissionsNoAddGrant.AddGrant = false

	BeforeEach(func() {
		manager = &mocks.Manager{}

		registry.Register("mockManager", func(m map[string]interface{}) (share.Manager, error) {
			return manager, nil
		})
		manager.On("UpdateShare", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&collaborationpb.Share{}, nil)

		gatewayClient = &cs3mocks.GatewayAPIClient{}
		pool.RemoveSelector("GatewaySelector" + "any")
		gatewaySelector = pool.GetSelector[gateway.GatewayAPIClient](
			"GatewaySelector",
			"any",
			func(cc *grpc.ClientConn) gateway.GatewayAPIClient {
				return gatewayClient
			},
		)
		checkPermissionResponse = &permissions.CheckPermissionResponse{
			Status: status.NewOK(ctx),
		}
		gatewayClient.On("CheckPermission", mock.Anything, mock.Anything).
			Return(checkPermissionResponse, nil)

		statResourceResponse = &providerpb.StatResponse{
			Status: status.NewOK(ctx),
			Info: &providerpb.ResourceInfo{
				PermissionSet: &providerpb.ResourcePermissions{},
			},
		}
		gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(statResourceResponse, nil)
		alice := &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		}

		getShareResponse = &collaborationpb.Share{
			Id: &collaborationpb.ShareId{
				OpaqueId: "shareid",
			},
			ResourceId: &providerpb.ResourceId{
				StorageId: "storageid",
				SpaceId:   "spaceid",
				OpaqueId:  "opaqueid",
			},
			Owner:   alice.Id,
			Creator: alice.Id,
		}
		manager.On("GetShare", mock.Anything, mock.Anything).Return(getShareResponse, nil)

		rgrpcService := usershareprovider.New(gatewaySelector, manager, []*regexp.Regexp{})

		provider = rgrpcService.(collaborationpb.CollaborationAPIServer)
		Expect(provider).ToNot(BeNil())

		ctx = ctxpkg.ContextSetUser(context.Background(), alice)
	})

	Describe("CreateShare", func() {
		DescribeTable("only requests with sufficient permissions get passed to the manager",
			func(
				resourceInfoPermissions *providerpb.ResourcePermissions,
				grantPermissions *providerpb.ResourcePermissions,
				checkPermissionStatusCode rpcpb.Code,
				expectedCode rpcpb.Code,
				expectedCalls int,
			) {
				manager.On("Share", mock.Anything, mock.Anything, mock.Anything).Return(&collaborationpb.Share{}, nil)
				checkPermissionResponse.Status.Code = checkPermissionStatusCode

				statResourceResponse.Info.PermissionSet = resourceInfoPermissions

				createShareResponse, err := provider.CreateShare(ctx, &collaborationpb.CreateShareRequest{
					ResourceInfo: &providerpb.ResourceInfo{
						PermissionSet: resourceInfoPermissions,
					},
					Grant: &collaborationpb.ShareGrant{
						Permissions: &collaborationpb.SharePermissions{
							Permissions: grantPermissions,
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(createShareResponse.Status.Code).To(Equal(expectedCode))

				manager.AssertNumberOfCalls(GinkgoT(), "Share", expectedCalls)
			},
			Entry(
				"insufficient permissions",
				conversions.RoleFromName("spaceeditor").CS3ResourcePermissions(),
				conversions.RoleFromName("manager").CS3ResourcePermissions(),
				rpcpb.Code_CODE_OK,
				rpcpb.Code_CODE_INVALID_ARGUMENT,
				0,
			),
			Entry(
				"sufficient permissions",
				conversions.RoleFromName("manager").CS3ResourcePermissions(),
				conversions.RoleFromName("spaceeditor").CS3ResourcePermissions(),
				rpcpb.Code_CODE_OK,
				rpcpb.Code_CODE_OK,
				1,
			),
			Entry(
				"no AddGrant permission on resource",
				cs3permissionsNoAddGrant,
				conversions.RoleFromName("spaceeditor").CS3ResourcePermissions(),
				rpcpb.Code_CODE_OK,
				rpcpb.Code_CODE_PERMISSION_DENIED,
				0,
			),
			Entry(
				"no WriteShare permission on user role",
				conversions.RoleFromName("manager").CS3ResourcePermissions(),
				conversions.RoleFromName("mspaceeditor").CS3ResourcePermissions(),
				rpcpb.Code_CODE_PERMISSION_DENIED,
				rpcpb.Code_CODE_PERMISSION_DENIED,
				0,
			),
		)
		Context("resharing is not allowed", func() {
			JustBeforeEach(func() {
				rgrpcService := usershareprovider.New(gatewaySelector, manager, []*regexp.Regexp{})

				provider = rgrpcService.(collaborationpb.CollaborationAPIServer)
				Expect(provider).ToNot(BeNil())

				// user has list grants access
				statResourceResponse.Info.PermissionSet = &providerpb.ResourcePermissions{
					AddGrant:   true,
					ListGrants: true,
				}
			})
			DescribeTable("rejects shares with any grant changing permissions",
				func(
					resourceInfoPermissions *providerpb.ResourcePermissions,
					grantPermissions *providerpb.ResourcePermissions,
					responseCode rpcpb.Code,
					expectedCalls int,
				) {
					manager.On("Share", mock.Anything, mock.Anything, mock.Anything).Return(&collaborationpb.Share{}, nil)

					createShareResponse, err := provider.CreateShare(ctx, &collaborationpb.CreateShareRequest{
						ResourceInfo: &providerpb.ResourceInfo{
							PermissionSet: resourceInfoPermissions,
						},
						Grant: &collaborationpb.ShareGrant{
							Permissions: &collaborationpb.SharePermissions{
								Permissions: grantPermissions,
							},
						},
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(createShareResponse.Status.Code).To(Equal(responseCode))

					manager.AssertNumberOfCalls(GinkgoT(), "Share", expectedCalls)
				},
				Entry("AddGrant", conversions.RoleFromName("manager").CS3ResourcePermissions(), &providerpb.ResourcePermissions{AddGrant: true}, rpcpb.Code_CODE_INVALID_ARGUMENT, 0),
				Entry("UpdateGrant", conversions.RoleFromName("manager").CS3ResourcePermissions(), &providerpb.ResourcePermissions{UpdateGrant: true}, rpcpb.Code_CODE_INVALID_ARGUMENT, 0),
				Entry("RemoveGrant", conversions.RoleFromName("manager").CS3ResourcePermissions(), &providerpb.ResourcePermissions{RemoveGrant: true}, rpcpb.Code_CODE_INVALID_ARGUMENT, 0),
				Entry("DenyGrant", conversions.RoleFromName("manager").CS3ResourcePermissions(), &providerpb.ResourcePermissions{DenyGrant: true}, rpcpb.Code_CODE_INVALID_ARGUMENT, 0),
				Entry("ListGrants", conversions.RoleFromName("manager").CS3ResourcePermissions(), &providerpb.ResourcePermissions{ListGrants: true}, rpcpb.Code_CODE_OK, 1),
			)
		})
	})
	Describe("UpdateShare", func() {
		It("fails without WriteShare permission in user role", func() {
			checkPermissionResponse.Status.Code = rpcpb.Code_CODE_PERMISSION_DENIED

			updateShareResponse, err := provider.UpdateShare(ctx, &collaborationpb.UpdateShareRequest{
				Ref: &collaborationpb.ShareReference{
					Spec: &collaborationpb.ShareReference_Id{
						Id: &collaborationpb.ShareId{
							OpaqueId: "shareid",
						},
					},
				},
				Share: &collaborationpb.Share{},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"permissions"},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(updateShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_PERMISSION_DENIED))

			manager.AssertNumberOfCalls(GinkgoT(), "UpdateShare", 0)
		})
		It("fails when the user tries to share with elevated permissions", func() {
			// user has only read access
			statResourceResponse.Info.PermissionSet = &providerpb.ResourcePermissions{
				InitiateFileDownload: true,
				Stat:                 true,
			}

			// user tries to update a share to give write permissions
			updateShareResponse, err := provider.UpdateShare(ctx, &collaborationpb.UpdateShareRequest{
				Ref: &collaborationpb.ShareReference{
					Spec: &collaborationpb.ShareReference_Id{
						Id: &collaborationpb.ShareId{
							OpaqueId: "shareid",
						},
					},
				},
				Share: &collaborationpb.Share{
					Permissions: &collaborationpb.SharePermissions{
						Permissions: &providerpb.ResourcePermissions{
							Stat:                 true,
							InitiateFileDownload: true,
							InitiateFileUpload:   true,
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"permissions"},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(updateShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_PERMISSION_DENIED))

			manager.AssertNumberOfCalls(GinkgoT(), "UpdateShare", 0)
		})
		It("succeeds when the user is not the owner/creator and does not have the UpdateGrant permissions", func() {
			// user has only read access
			statResourceResponse.Info.PermissionSet = &providerpb.ResourcePermissions{
				InitiateFileDownload: true,
				Stat:                 true,
			}
			bobId := &userpb.UserId{OpaqueId: "bob"}
			getShareResponse.Owner = bobId
			getShareResponse.Creator = bobId

			// user tries to update a share to give write permissions
			updateShareResponse, err := provider.UpdateShare(ctx, &collaborationpb.UpdateShareRequest{
				Ref: &collaborationpb.ShareReference{
					Spec: &collaborationpb.ShareReference_Id{
						Id: &collaborationpb.ShareId{
							OpaqueId: "shareid",
						},
					},
				},
				Share: &collaborationpb.Share{
					Permissions: &collaborationpb.SharePermissions{
						Permissions: &providerpb.ResourcePermissions{
							Stat:                 true,
							InitiateFileDownload: true,
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"permissions"},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(updateShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_PERMISSION_DENIED))

			manager.AssertNumberOfCalls(GinkgoT(), "UpdateShare", 0)
		})
		It("succeeds when the user is the owner/creator", func() {
			// user has only read access
			statResourceResponse.Info.PermissionSet = &providerpb.ResourcePermissions{
				InitiateFileDownload: true,
				Stat:                 true,
			}

			// user tries to update a share to give write permissions
			updateShareResponse, err := provider.UpdateShare(ctx, &collaborationpb.UpdateShareRequest{
				Ref: &collaborationpb.ShareReference{
					Spec: &collaborationpb.ShareReference_Id{
						Id: &collaborationpb.ShareId{
							OpaqueId: "shareid",
						},
					},
				},
				Share: &collaborationpb.Share{
					Permissions: &collaborationpb.SharePermissions{
						Permissions: &providerpb.ResourcePermissions{
							Stat:                 true,
							InitiateFileDownload: true,
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"permissions"},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(updateShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_OK))

			manager.AssertNumberOfCalls(GinkgoT(), "UpdateShare", 1)
		})
		It("succeeds when the user is not the owner/creator but has the UpdateGrant permissions", func() {
			// user has only read access
			statResourceResponse.Info.PermissionSet = &providerpb.ResourcePermissions{
				UpdateGrant:          true,
				InitiateFileDownload: true,
				Stat:                 true,
			}
			bobId := &userpb.UserId{OpaqueId: "bob"}
			getShareResponse.Owner = bobId
			getShareResponse.Creator = bobId

			// user tries to update a share to give write permissions
			updateShareResponse, err := provider.UpdateShare(ctx, &collaborationpb.UpdateShareRequest{
				Ref: &collaborationpb.ShareReference{
					Spec: &collaborationpb.ShareReference_Id{
						Id: &collaborationpb.ShareId{
							OpaqueId: "shareid",
						},
					},
				},
				Share: &collaborationpb.Share{
					Permissions: &collaborationpb.SharePermissions{
						Permissions: &providerpb.ResourcePermissions{
							Stat:                 true,
							InitiateFileDownload: true,
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"permissions"},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(updateShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_OK))

			manager.AssertNumberOfCalls(GinkgoT(), "UpdateShare", 1)
		})
	})
})
