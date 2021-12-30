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

package publicstorageprovider

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	gatewayMock "github.com/cs3org/go-cs3apis/mocks/cs3/gateway/v1beta1"
)

var (
	publicStorageProviderID = "e1a73ede-549b-4226-abdf-40e69ca8230d"
)

var _ = Describe("Public Storage Provider", func() {
	var (
		ctx context.Context
		gwc *gatewayMock.GatewayAPIClient
		psp *service
	)

	BeforeEach(func() {
		ctx = context.Background()
		gwc = &gatewayMock.GatewayAPIClient{}
		psp = &service{
			conf:      &config{},
			mountPath: "/public/",
			mountID:   publicStorageProviderID,
			gateway:   gwc,
		}

	})

	AfterEach(func() {

	})

	Describe("When a user stats the root of a public link", func() {

		Context("by path", func() {
			It("returns the root", func() {
				// mocks
				gwc.Mock.On(
					"GetPublicShare",
					mock.Anything,
					&linkv1beta1.GetPublicShareRequest{
						Ref: &linkv1beta1.PublicShareReference{
							Spec: &linkv1beta1.PublicShareReference_Token{
								Token: "public-token-123",
							},
						},
						Sign: true,
					},
				).Return(
					&linkv1beta1.GetPublicShareResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Share: &linkv1beta1.PublicShare{
							Id: &linkv1beta1.PublicShareId{
								OpaqueId: "omg",
							},
							Token: "public-token-123",
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Permissions: &linkv1beta1.PublicSharePermissions{
								Permissions: &providerv1beta1.ResourcePermissions{
									Stat: true,
								},
							},
						},
					},
					nil,
				).Once()

				gwc.Mock.On(
					"Stat",
					mock.Anything,
					&providerv1beta1.StatRequest{
						Ref: &providerv1beta1.Reference{
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Path: "",
						},
					},
				).Return(
					&providerv1beta1.StatResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Info: &providerv1beta1.ResourceInfo{
							Type: providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
							Id: &providerv1beta1.ResourceId{
								StorageId: "real-storage",
								OpaqueId:  "real-file-id",
							},
							PermissionSet: &providerv1beta1.ResourcePermissions{},
						},
					},
					nil,
				).Once()

				// the actual request
				req := &providerv1beta1.StatRequest{
					Ref: &providerv1beta1.Reference{
						Path: "/public/public-token-123/",
					},
				}

				resp, err := psp.Stat(ctx, req)

				// check response
				Expect(err).To(BeNil())
				Expect(resp.Info.Path).To(Equal("/public/public-token-123"))
			})
		})

		Context("stat the file by id", func() {
			It("returns the root", func() {

				// mocks
				gwc.Mock.On(
					"GetPublicShare",
					mock.Anything,
					&linkv1beta1.GetPublicShareRequest{
						Ref: &linkv1beta1.PublicShareReference{
							Spec: &linkv1beta1.PublicShareReference_Token{
								Token: "public-token-123",
							},
						},
						Sign: true,
					},
				).Return(
					&linkv1beta1.GetPublicShareResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Share: &linkv1beta1.PublicShare{
							Id: &linkv1beta1.PublicShareId{
								OpaqueId: "omg",
							},
							Token: "public-token-123",
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Permissions: &linkv1beta1.PublicSharePermissions{
								Permissions: &providerv1beta1.ResourcePermissions{
									Stat: true,
								},
							},
						},
					},
					nil,
				).Once()

				gwc.Mock.On(
					"Stat",
					mock.Anything,
					&providerv1beta1.StatRequest{
						Ref: &providerv1beta1.Reference{
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Path: "",
						},
					},
				).Return(
					&providerv1beta1.StatResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Info: &providerv1beta1.ResourceInfo{
							Type: providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
							Id: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							PermissionSet: &providerv1beta1.ResourcePermissions{},
						},
					},
					nil,
				)

				gwc.Mock.On(
					"Stat",
					mock.Anything,
					&providerv1beta1.StatRequest{
						Ref: &providerv1beta1.Reference{
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Path: "",
						},
					},
				).Return(
					&providerv1beta1.StatResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Info: &providerv1beta1.ResourceInfo{
							Type: providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
							Id: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							PermissionSet: &providerv1beta1.ResourcePermissions{},
						},
					},
					nil,
				).Once()

				// the actual request
				req := &providerv1beta1.StatRequest{
					Ref: &providerv1beta1.Reference{
						ResourceId: &providerv1beta1.ResourceId{
							StorageId: publicStorageProviderID,
							OpaqueId:  "public-token-123/file-id",
						},
					},
				}

				resp, err := psp.Stat(ctx, req)

				// check response
				Expect(err).To(BeNil())
				Expect(resp.Info.Path).To(Equal("/public/public-token-123"))
			})
		})
	})
})
