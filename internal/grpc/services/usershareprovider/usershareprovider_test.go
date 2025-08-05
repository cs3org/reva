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
	"path/filepath"
	"regexp"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationpb "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/opencloud-eu/reva/v2/internal/grpc/services/usershareprovider"
	"github.com/opencloud-eu/reva/v2/pkg/conversions"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/status"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/opencloud-eu/reva/v2/pkg/share"
	_ "github.com/opencloud-eu/reva/v2/pkg/share/manager/loader"
	"github.com/opencloud-eu/reva/v2/pkg/share/manager/registry"
	"github.com/opencloud-eu/reva/v2/pkg/share/mocks"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	cs3mocks "github.com/opencloud-eu/reva/v2/tests/cs3mocks/mocks"
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

		alice = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
				TenantId: "tenant1",
			},
			Username: "alice",
		}

		bob = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "bob",
				TenantId: "tenant1",
			},
			Username: "bob",
		}

		carol = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "carol",
				TenantId: "tenant2",
			},
			Username: "carol",
		}
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
			func(cc grpc.ClientConnInterface) gateway.GatewayAPIClient {
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

	Describe("UpdateReceivedShare", func() {
		DescribeTable("validates the share update request",
			func(
				req *collaborationpb.UpdateReceivedShareRequest,
				expectedStatus *rpcpb.Status,
				expectedError error,
			) {

				res, err := provider.UpdateReceivedShare(ctx, req)

				switch expectedError {
				case nil:
					Expect(err).To(BeNil())
				}

				Expect(res.GetStatus().GetCode()).To(Equal(expectedStatus.GetCode()))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring(expectedStatus.GetMessage()))
			},
			Entry(
				"no share opaque id",
				&collaborationpb.UpdateReceivedShareRequest{
					Share: &collaborationpb.ReceivedShare{
						Share: &collaborationpb.Share{
							Id: &collaborationpb.ShareId{},
						},
					},
				},
				status.NewInvalid(ctx, "share id empty"),
				nil,
			),
		)

		DescribeTable("fails if getting the share fails",
			func(
				req *collaborationpb.UpdateReceivedShareRequest,
				expectedStatus *rpcpb.Status,
				expectedError error,
			) {
				gatewayClient.EXPECT().
					GetReceivedShare(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, request *collaborationpb.GetReceivedShareRequest, option ...grpc.CallOption) (*collaborationpb.GetReceivedShareResponse, error) {
						return &collaborationpb.GetReceivedShareResponse{
							Status: expectedStatus,
						}, expectedError
					})

				res, err := provider.UpdateReceivedShare(ctx, req)

				switch expectedError {
				case nil:
					Expect(err).To(BeNil())
				default:
					Expect(err).To(MatchError(expectedError))
				}

				switch expectedStatus {
				case nil:
					Expect(expectedStatus).To(BeNil())
				default:
					Expect(res.GetStatus().GetCode()).To(Equal(expectedStatus.GetCode()))
					Expect(res.GetStatus().GetMessage()).To(ContainSubstring(expectedStatus.GetMessage()))
				}
			},
			Entry(
				"requesting the share errors",
				&collaborationpb.UpdateReceivedShareRequest{
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"state"},
					},
					Share: &collaborationpb.ReceivedShare{
						State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaborationpb.Share{
							Id: &collaborationpb.ShareId{
								OpaqueId: "1",
							},
						},
					},
				},
				nil,
				errors.New("some"),
			),
			Entry(
				"requesting the share fails",
				&collaborationpb.UpdateReceivedShareRequest{
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"state"},
					},
					Share: &collaborationpb.ReceivedShare{
						State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaborationpb.Share{
							Id: &collaborationpb.ShareId{
								OpaqueId: "1",
							},
						},
					},
				},
				status.NewInvalid(ctx, "something"),
				nil,
			),
		)

		DescribeTable("fails if the resource stat fails",
			func(
				req *collaborationpb.UpdateReceivedShareRequest,
				expectedStatus *rpcpb.Status,
				expectedError error,
			) {
				gatewayClient.EXPECT().
					GetReceivedShare(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, request *collaborationpb.GetReceivedShareRequest, option ...grpc.CallOption) (*collaborationpb.GetReceivedShareResponse, error) {
						return &collaborationpb.GetReceivedShareResponse{
							Status: status.NewOK(ctx),
						}, nil
					})
				gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything, mock.Anything).Unset()
				gatewayClient.EXPECT().
					Stat(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, request *providerpb.StatRequest, option ...grpc.CallOption) (*providerpb.StatResponse, error) {
						return &providerpb.StatResponse{
							Status: expectedStatus,
						}, expectedError
					})

				res, err := provider.UpdateReceivedShare(ctx, req)

				switch expectedError {
				case nil:
					Expect(err).To(BeNil())
				default:
					Expect(err).To(MatchError(expectedError))
				}

				switch expectedStatus {
				case nil:
					Expect(expectedStatus).To(BeNil())
				default:
					Expect(res.GetStatus().GetCode()).To(Equal(expectedStatus.GetCode()))
					Expect(res.GetStatus().GetMessage()).To(ContainSubstring(expectedStatus.GetMessage()))
				}
			},
			Entry(
				"stat the resource errors",
				&collaborationpb.UpdateReceivedShareRequest{
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"state"},
					},
					Share: &collaborationpb.ReceivedShare{
						State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaborationpb.Share{
							Id: &collaborationpb.ShareId{
								OpaqueId: "1",
							},
						},
					},
				},
				nil,
				errors.New("some"),
			),
			Entry(
				"stat the resource fails",
				&collaborationpb.UpdateReceivedShareRequest{
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"state"},
					},
					Share: &collaborationpb.ReceivedShare{
						State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaborationpb.Share{
							Id: &collaborationpb.ShareId{
								OpaqueId: "1",
							},
						},
					},
				},
				status.NewInvalid(ctx, "something"),
				nil,
			),
		)
	})

	Describe("CreateShare", func() {
		BeforeEach(func() {
			manager.On("Share", mock.Anything, mock.Anything, mock.Anything).Return(&collaborationpb.Share{}, nil)
		})

		DescribeTable("only requests with sufficient permissions get passed to the manager",
			func(
				resourceInfoPermissions *providerpb.ResourcePermissions,
				grantPermissions *providerpb.ResourcePermissions,
				checkPermissionStatusCode rpcpb.Code,
				expectedCode rpcpb.Code,
				expectedCalls int,
			) {
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
		Context("create share with tenant awareness", func() {
			JustBeforeEach(func() {
				rgrpcService := usershareprovider.New(gatewaySelector, manager, []*regexp.Regexp{})

				provider = rgrpcService.(collaborationpb.CollaborationAPIServer)
				Expect(provider).ToNot(BeNil())

				// user has list grants access
				statResourceResponse.Info.PermissionSet = conversions.RoleFromName(conversions.RoleCoowner).CS3ResourcePermissions()
			})

			It("fails when the tenantId of the user does not match the tenantId of the target user", func() {
				createShareResponse, err := provider.CreateShare(ctx, &collaborationpb.CreateShareRequest{
					ResourceInfo: &providerpb.ResourceInfo{
						PermissionSet: conversions.RoleFromName("manager").CS3ResourcePermissions(),
					},
					Grant: &collaborationpb.ShareGrant{
						Grantee: &providerpb.Grantee{
							Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
							Id:   &providerpb.Grantee_UserId{UserId: carol.GetId()},
						},
						Permissions: &collaborationpb.SharePermissions{
							Permissions: conversions.RoleFromName("spaceeditor").CS3ResourcePermissions(),
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(createShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_PERMISSION_DENIED))

				manager.AssertNumberOfCalls(GinkgoT(), "Share", 0)
			})

			It("succeeds when the tenantId of the user matches the tenantId of the target user", func() {

				createShareResponse, err := provider.CreateShare(ctx, &collaborationpb.CreateShareRequest{
					ResourceInfo: &providerpb.ResourceInfo{
						PermissionSet: conversions.RoleFromName("manager").CS3ResourcePermissions(),
					},
					Grant: &collaborationpb.ShareGrant{
						Grantee: &providerpb.Grantee{
							Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
							Id:   &providerpb.Grantee_UserId{UserId: bob.GetId()},
						},
						Permissions: &collaborationpb.SharePermissions{
							Permissions: conversions.RoleFromName("viewer").CS3ResourcePermissions(),
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(createShareResponse.Status.Code).To(Equal(rpcpb.Code_CODE_OK))

				manager.AssertNumberOfCalls(GinkgoT(), "Share", 1)
			})
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
			getShareResponse.Owner = bob.GetId()
			getShareResponse.Creator = bob.GetId()

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
			getShareResponse.Owner = bob.GetId()
			getShareResponse.Creator = bob.GetId()

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

var _ = Describe("helpers", func() {
	type GetMountpointAndUnmountedSharesArgs struct {
		withName                   string
		withResourceId             *providerpb.ResourceId
		listReceivedSharesResponse *collaborationpb.ListReceivedSharesResponse
		listReceivedSharesError    error
		expectedName               string
	}
	DescribeTable("GetMountpointAndUnmountedShares",
		func(args GetMountpointAndUnmountedSharesArgs) {
			gatewayClient := cs3mocks.NewGatewayAPIClient(GinkgoT())

			gatewayClient.EXPECT().
				ListReceivedShares(mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, request *collaborationpb.ListReceivedSharesRequest, option ...grpc.CallOption) (*collaborationpb.ListReceivedSharesResponse, error) {
					return args.listReceivedSharesResponse, args.listReceivedSharesError
				})

			statCallCount := 0

			for _, s := range args.listReceivedSharesResponse.GetShares() {
				if s.GetState() != collaborationpb.ShareState_SHARE_STATE_ACCEPTED {
					continue
				}

				// add one for every accepted share where the resource id matches
				if utils.ResourceIDEqual(s.GetShare().GetResourceId(), args.withResourceId) {
					statCallCount++
				}

				// add one for every accepted share where the mountpoint patch matches
				if s.GetMountPoint().GetPath() == filepath.Clean(args.withName) {
					statCallCount++
				}
			}

			if statCallCount > 0 {
				gatewayClient.EXPECT().
					Stat(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, request *providerpb.StatRequest, option ...grpc.CallOption) (*providerpb.StatResponse, error) {
						return &providerpb.StatResponse{
							Status: status.NewOK(ctx),
						}, nil
					}).Times(statCallCount)
			}

			availableMountpoint, _, err := usershareprovider.GetMountpointAndUnmountedShares(context.Background(), gatewayClient, args.withResourceId, args.withName, nil)

			if args.listReceivedSharesError != nil {
				Expect(err).To(HaveOccurred(), "expected error, got none")
				return
			}

			Expect(availableMountpoint).To(Equal(args.expectedName), "expected mountpoint %s, got %s", args.expectedName, availableMountpoint)

			gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything, mock.Anything).Unset()
		},
		Entry(
			"listing received shares errors",
			GetMountpointAndUnmountedSharesArgs{
				listReceivedSharesError: errors.New("some error"),
			},
		),
		Entry(
			"returns the given name if no shares are found",
			GetMountpointAndUnmountedSharesArgs{
				withName: "name1",
				listReceivedSharesResponse: &collaborationpb.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
				},
				expectedName: "name1",
			},
		),
		Entry(
			"returns the path as name if a share with the same resourceId is found",
			GetMountpointAndUnmountedSharesArgs{
				withName: "name",
				withResourceId: &providerpb.ResourceId{
					StorageId: "1",
					OpaqueId:  "2",
					SpaceId:   "3",
				},
				listReceivedSharesResponse: &collaborationpb.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaborationpb.ReceivedShare{
						{
							State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
							MountPoint: &providerpb.Reference{
								Path: "path",
							},
							Share: &collaborationpb.Share{
								ResourceId: &providerpb.ResourceId{
									StorageId: "1",
									OpaqueId:  "2",
									SpaceId:   "3",
								},
							},
						},
					},
				},
				expectedName: "path",
			},
		),
		Entry(
			"enumerates the name if a share with the same path already exists",
			GetMountpointAndUnmountedSharesArgs{
				withName: "some name",
				listReceivedSharesResponse: &collaborationpb.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaborationpb.ReceivedShare{
						{
							State: collaborationpb.ShareState_SHARE_STATE_ACCEPTED,
							MountPoint: &providerpb.Reference{
								Path: "some name",
							},
							Share: &collaborationpb.Share{
								ResourceId: &providerpb.ResourceId{
									StorageId: "1",
									OpaqueId:  "2",
									SpaceId:   "3",
								},
							},
						},
					},
				},
				expectedName: "some name (1)",
			},
		),
	)
})
