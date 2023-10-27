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

package sharesstorageprovider_test

import (
	"context"
	"os"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	sprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	provider "github.com/cs3org/reva/v2/internal/grpc/services/sharesstorageprovider"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	_ "github.com/cs3org/reva/v2/pkg/share/manager/loader"
	"github.com/cs3org/reva/v2/pkg/utils"
	cs3mocks "github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var (
	ShareJail                *sprovider.ResourceId
	BaseShare                *collaboration.ReceivedShare
	BaseShareTwo             *collaboration.ReceivedShare
	ShareJailStatRequest     *sprovider.StatRequest
	BaseStatRequest          *sprovider.StatRequest
	BaseListContainerRequest *sprovider.ListContainerRequest
)

var _ = Describe("Sharesstorageprovider", func() {
	var (
		config = map[string]interface{}{
			"gateway_addr":         "127.0.0.1:1234",
			"driver":               "json",
			"usershareprovidersvc": "any",
			"drivers": map[string]map[string]interface{}{
				"json": {},
			},
		}
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		})

		s                            sprovider.ProviderAPIServer
		gatewayClient                *cs3mocks.GatewayAPIClient
		gatewaySelector              pool.Selectable[gateway.GatewayAPIClient]
		sharingCollaborationClient   *cs3mocks.CollaborationAPIClient
		sharingCollaborationSelector pool.Selectable[collaboration.CollaborationAPIClient]
	)

	BeforeEach(func() {
		ShareJail = &sprovider.ResourceId{
			StorageId: utils.ShareStorageProviderID,
			SpaceId:   utils.ShareStorageSpaceID,
			OpaqueId:  utils.ShareStorageSpaceID,
		}

		BaseShare = &collaboration.ReceivedShare{
			State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			Share: &collaboration.Share{
				Id: &collaboration.ShareId{
					OpaqueId: "shareid",
				},
				ResourceId: &sprovider.ResourceId{
					StorageId: "share1-storageid",
					SpaceId:   "share1-storageid",
					OpaqueId:  "shareddir",
				},
				Permissions: &collaboration.SharePermissions{
					Permissions: &sprovider.ResourcePermissions{
						Stat:               true,
						ListContainer:      true,
						InitiateFileUpload: true,
					},
				},
			},
			MountPoint: &sprovider.Reference{
				Path: "oldname",
			},
		}

		BaseShareTwo = &collaboration.ReceivedShare{
			State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			Share: &collaboration.Share{
				Id: &collaboration.ShareId{
					OpaqueId: "shareidtwo",
				},
				ResourceId: &sprovider.ResourceId{
					StorageId: "share1-storageid",
					SpaceId:   "share1-storageid",
					OpaqueId:  "shareddir",
				},
				Permissions: &collaboration.SharePermissions{
					Permissions: &sprovider.ResourcePermissions{
						Stat:               true,
						ListContainer:      true,
						InitiateFileUpload: true,
					},
				},
			},
			MountPoint: &sprovider.Reference{
				Path: "share1-shareddir",
			},
		}

		ShareJailStatRequest = &sprovider.StatRequest{
			Ref: &sprovider.Reference{
				ResourceId: &sprovider.ResourceId{
					StorageId: utils.ShareStorageProviderID,
					SpaceId:   utils.ShareStorageSpaceID,
					OpaqueId:  utils.ShareStorageSpaceID,
				},
				Path: ".",
			},
		}

		BaseStatRequest = &sprovider.StatRequest{
			Ref: &sprovider.Reference{
				ResourceId: &sprovider.ResourceId{
					StorageId: utils.ShareStorageProviderID,
					SpaceId:   utils.ShareStorageSpaceID,
					OpaqueId:  "shareid",
				},
				Path: ".",
			},
		}

		BaseListContainerRequest = &sprovider.ListContainerRequest{
			Ref: &sprovider.Reference{
				ResourceId: &sprovider.ResourceId{
					StorageId: utils.ShareStorageProviderID,
					SpaceId:   utils.ShareStorageSpaceID,
					OpaqueId:  "shareid",
				},
				Path: ".",
			},
		}

		pool.RemoveSelector("GatewaySelector" + "any")
		gatewayClient = &cs3mocks.GatewayAPIClient{}
		gatewaySelector = pool.GetSelector[gateway.GatewayAPIClient](
			"GatewaySelector",
			"any",
			func(cc *grpc.ClientConn) gateway.GatewayAPIClient {
				return gatewayClient
			},
		)

		pool.RemoveSelector("SharingCollaborationSelector" + "any")
		sharingCollaborationClient = &cs3mocks.CollaborationAPIClient{}
		sharingCollaborationSelector = pool.GetSelector[collaboration.CollaborationAPIClient](
			"SharingCollaborationSelector",
			"any",
			func(cc *grpc.ClientConn) collaboration.CollaborationAPIClient {
				return sharingCollaborationClient
			},
		)

		// mock stat requests
		// some-provider-id
		gatewayClient.On("Stat", mock.Anything, mock.AnythingOfType("*providerv1beta1.StatRequest")).Return(
			func(_ context.Context, req *sprovider.StatRequest, _ ...grpc.CallOption) *sprovider.StatResponse {
				switch req.Ref.GetPath() {
				case "./share1-shareddir", "./share1-shareddir (1)":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "share1-shareddir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "shareddir",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 100,
						},
					}
				case "./share1-subdir":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "share1-subdir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "subdir",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 10,
						},
					}
				case "./share1-subdir/share1-subdir-file":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_FILE,
							Path: "share1-subdir-file",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "file",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 20,
						},
					}
				case ".":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "share1-shareddir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "shareddir",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 100,
						},
					}
				default:
					return &sprovider.StatResponse{
						Status: status.NewNotFound(context.Background(), "not found"),
					}
				}
			},
			nil)

		gatewayClient.On("ListContainer", mock.Anything, mock.AnythingOfType("*providerv1beta1.ListContainerRequest")).Return(
			func(_ context.Context, req *sprovider.ListContainerRequest, _ ...grpc.CallOption) *sprovider.ListContainerResponse {
				switch {
				case utils.ResourceIDEqual(req.Ref.ResourceId, BaseShare.Share.ResourceId):
					resp := &sprovider.ListContainerResponse{
						Status: status.NewOK(context.Background()),
						Infos:  []*sprovider.ResourceInfo{},
					}

					switch req.Ref.GetPath() {
					case ".":
						resp.Infos = append(resp.Infos, &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "share1-shareddir/share1-subdir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "subdir",
							},
							Size: 1,
						})
					case "./share1-subdir":
						resp.Infos = append(resp.Infos, &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "share1-shareddir/share1-subdir/share1-subdir-file",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								SpaceId:   "share1-storageid",
								OpaqueId:  "file",
							},
							Size: 1,
						})
					}
					return resp
				case utils.ResourceIDEqual(req.Ref.ResourceId, BaseShareTwo.Share.ResourceId):
					return &sprovider.ListContainerResponse{
						Status: status.NewOK(context.Background()),
						Infos:  []*sprovider.ResourceInfo{},
					}
				default:
					return &sprovider.ListContainerResponse{
						Status: status.NewOK(context.Background()),
						Infos:  []*sprovider.ResourceInfo{},
					}
				}
			}, nil)

	})

	JustBeforeEach(func() {
		p, err := provider.New(gatewaySelector, sharingCollaborationSelector)
		Expect(err).ToNot(HaveOccurred())
		s = p.(sprovider.ProviderAPIServer)
		Expect(s).ToNot(BeNil())
	})

	Describe("NewDefault", func() {
		It("returns a new service instance", func() {
			tmpfile, err := os.CreateTemp("", "eos-unit-test-shares-*.json")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(tmpfile.Name())

			config["drivers"] = map[string]map[string]interface{}{
				"json": {
					"file":     tmpfile.Name(),
					"mount_id": "shareprovidermountid",
				},
			}
			s, err := provider.NewDefault(config, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(s).ToNot(BeNil())
		})
	})

	Describe("ListContainer", func() {
		It("lists accepted shares", func() {
			sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: BaseShare.Share,
					State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
				},
			}, nil)
			res, err := s.ListContainer(ctx, BaseListContainerRequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(1))
		})
		It("ignores invalid shares", func() {
			sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
					State: collaboration.ShareState_SHARE_STATE_INVALID,
				},
			}, nil)
			res, err := s.ListContainer(ctx, BaseListContainerRequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(0))
		})
		It("ignores pending shares", func() {
			sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
					State: collaboration.ShareState_SHARE_STATE_PENDING,
				},
			}, nil)
			res, err := s.ListContainer(ctx, BaseListContainerRequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(0))
		})
		It("ignores rejected shares", func() {
			sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(&collaboration.GetReceivedShareResponse{
				Status: status.NewOK(context.Background()),
				Share: &collaboration.ReceivedShare{
					Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
					State: collaboration.ShareState_SHARE_STATE_REJECTED,
				},
			}, nil)
			res, err := s.ListContainer(ctx, BaseListContainerRequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(0))
		})
	})

	Context("with two accepted shares", func() {
		BeforeEach(func() {
			sharingCollaborationClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{BaseShare, BaseShareTwo},
			}, nil)
			sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(
				func(_ context.Context, req *collaboration.GetReceivedShareRequest, _ ...grpc.CallOption) *collaboration.GetReceivedShareResponse {
					switch req.Ref.GetId().GetOpaqueId() {
					case BaseShare.Share.Id.OpaqueId:
						return &collaboration.GetReceivedShareResponse{
							Status: status.NewOK(context.Background()),
							Share:  BaseShare,
						}
					case BaseShareTwo.Share.Id.OpaqueId:
						return &collaboration.GetReceivedShareResponse{
							Status: status.NewOK(context.Background()),
							Share:  BaseShareTwo,
						}
					default:
						return &collaboration.GetReceivedShareResponse{
							Status: status.NewNotFound(context.Background(), "not found"),
						}
					}
				}, nil)
		})

		Describe("Stat", func() {
			It("stats the first share folder", func() {
				req := ShareJailStatRequest
				req.Ref.Path = "./share1-shareddir"
				res, err := s.Stat(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("share1-shareddir"))
				// Expect(res.Info.Size).To(Equal(uint64(300))) TODO: Why 300?
				Expect(res.Info.Size).To(Equal(uint64(100)))
			})

			It("stats the correct share in the share jail", func() {
				BaseShare.MountPoint.Path = "share1-shareddir"
				BaseShareTwo.MountPoint.Path = "share1-shareddir (1)"
				statReq := ShareJailStatRequest
				statReq.Ref.Path = "./share1-shareddir (1)"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("share1-shareddir"))
				Expect(res.Info.Size).To(Equal(uint64(100)))
			})

			It("stats a shares folder", func() {
				statReq := BaseStatRequest
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("share1-shareddir"))
				Expect(res.Info.Size).To(Equal(uint64(100)))
			})

			It("merges permissions from multiple shares", func() {
				s1 := &collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
					Share: &collaboration.Share{
						Id: &collaboration.ShareId{
							OpaqueId: "multishare1",
						},
						ResourceId: &sprovider.ResourceId{
							StorageId: "share1-storageid",
							SpaceId:   "share1-storageid",
							OpaqueId:  "shareddir",
						},
						Permissions: &collaboration.SharePermissions{
							Permissions: &sprovider.ResourcePermissions{
								Stat: true,
							},
						},
					},
					MountPoint: &sprovider.Reference{Path: "share1-shareddir"},
				}
				s2 := &collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
					Share: &collaboration.Share{
						Id: &collaboration.ShareId{
							OpaqueId: "multishare2",
						},
						ResourceId: &sprovider.ResourceId{
							StorageId: "share1-storageid",
							SpaceId:   "share1-storageid",
							OpaqueId:  "shareddir",
						},
						Permissions: &collaboration.SharePermissions{
							Permissions: &sprovider.ResourcePermissions{
								ListContainer: true,
							},
						},
					},
					MountPoint: &sprovider.Reference{Path: "share2-shareddir"},
				}

				sharingCollaborationClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{s1, s2},
				}, nil)
				sharingCollaborationClient.On("GetReceivedShare", mock.Anything, mock.Anything).Return(
					func(_ context.Context, req *collaboration.GetReceivedShareRequest, _ ...grpc.CallOption) *collaboration.GetReceivedShareResponse {
						switch req.Ref.GetId().GetOpaqueId() {
						case BaseShare.Share.Id.OpaqueId:
							return &collaboration.GetReceivedShareResponse{
								Status: status.NewOK(context.Background()),
								Share:  s1,
							}
						case BaseShareTwo.Share.Id.OpaqueId:
							return &collaboration.GetReceivedShareResponse{
								Status: status.NewOK(context.Background()),
								Share:  s2,
							}
						default:
							return &collaboration.GetReceivedShareResponse{
								Status: status.NewNotFound(context.Background(), "not found"),
							}
						}
					}, nil)

				res, err := s.Stat(ctx, BaseStatRequest)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("share1-shareddir"))
				Expect(res.Info.PermissionSet.Stat).To(BeTrue())
				// Expect(res.Info.PermissionSet.ListContainer).To(BeTrue()) // TODO reenable
			})

			It("stats a subfolder in a share", func() {
				statReq := BaseStatRequest
				statReq.Ref.Path = "./share1-subdir"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("share1-subdir"))
				Expect(res.Info.Size).To(Equal(uint64(10)))
			})

			It("stats a shared file", func() {
				statReq := BaseStatRequest
				statReq.Ref.Path = "./share1-subdir/share1-subdir-file"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_FILE))
				Expect(res.Info.Path).To(Equal("share1-subdir-file"))
				Expect(res.Info.Size).To(Equal(uint64(20)))
			})
		})

		Describe("ListContainer", func() {
			It("traverses into specific shares", func() {
				req := BaseListContainerRequest
				res, err := s.ListContainer(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Infos)).To(Equal(1))

				entry := res.Infos[0]
				Expect(entry.Path).To(Equal("share1-shareddir/share1-subdir"))
			})

			It("traverses into subfolders of specific shares", func() {
				req := BaseListContainerRequest
				req.Ref.Path = "./share1-subdir"
				res, err := s.ListContainer(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Infos)).To(Equal(1))

				entry := res.Infos[0]
				Expect(entry.Path).To(Equal("share1-shareddir/share1-subdir/share1-subdir-file"))
			})
		})

		Describe("InitiateFileDownload", func() {
			It("returns not found when not found", func() {
				gatewayClient.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found"),
				}, nil)

				req := &sprovider.InitiateFileDownloadRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/does-not-exist",
					},
				}
				res, err := s.InitiateFileDownload(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_NOT_FOUND))
			})

			It("initiates the download of an existing file", func() {
				gatewayClient.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
					Status: status.NewOK(ctx),
					Protocols: []*gateway.FileDownloadProtocol{
						{
							Opaque:           &types.Opaque{},
							Protocol:         "simple",
							DownloadEndpoint: "https://localhost:9200/data",
							Token:            "thetoken",
						},
					},
				}, nil)
				req := &sprovider.InitiateFileDownloadRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-subdir/share1-subdir-file",
					},
				}
				res, err := s.InitiateFileDownload(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Protocols[0].Protocol).To(Equal("simple"))
				Expect(res.Protocols[0].DownloadEndpoint).To(Equal("https://localhost:9200/data/thetoken"))
			})
		})

		Describe("CreateContainer", func() {
			BeforeEach(func() {
				gatewayClient.On("CreateContainer", mock.Anything, mock.Anything).Return(&sprovider.CreateContainerResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("refuses to create a top-level container which doesn't belong to a share", func() {
				req := &sprovider.CreateContainerRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/invalid-top-level-subdir",
					},
				}
				res, err := s.CreateContainer(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "CreateContainer", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("creates a directory", func() {
				req := &sprovider.CreateContainerRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-newsubdir",
					},
				}
				res, err := s.CreateContainer(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "CreateContainer", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})

		Describe("TouchFile", func() {
			BeforeEach(func() {
				gatewayClient.On("TouchFile", mock.Anything, mock.Anything).Return(
					&sprovider.TouchFileResponse{Status: status.NewOK(ctx)}, nil)
			})

			It("touches a file", func() {
				req := &sprovider.TouchFileRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/newfile.txt",
					},
				}
				res, err := s.TouchFile(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "TouchFile", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("Delete", func() {
			BeforeEach(func() {
				gatewayClient.On("Delete", mock.Anything, mock.Anything).Return(
					&sprovider.DeleteResponse{Status: status.NewOK(ctx)}, nil)
			})

			It("rejects the share when deleting a share", func() {
				sharingCollaborationClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(
					&collaboration.UpdateReceivedShareResponse{Status: status.NewOK(ctx)}, nil)
				req := &sprovider.DeleteRequest{
					Ref: &sprovider.Reference{
						ResourceId: &sprovider.ResourceId{
							StorageId: utils.ShareStorageProviderID,
							SpaceId:   utils.ShareStorageSpaceID,
							OpaqueId:  BaseShare.Share.Id.OpaqueId,
						},
						Path: ".",
					},
				}
				res, err := s.Delete(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))

				sharingCollaborationClient.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
			})

			It("deletes a file", func() {
				req := &sprovider.DeleteRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-subdir/share1-subdir-file",
					},
				}
				res, err := s.Delete(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("Move", func() {
			BeforeEach(func() {
				gatewayClient.On("Move", mock.Anything, mock.Anything).Return(&sprovider.MoveResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("renames a share", func() {
				sharingCollaborationClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(nil, nil)

				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						ResourceId: &sprovider.ResourceId{
							StorageId: utils.ShareStorageProviderID,
							SpaceId:   utils.ShareStorageSpaceID,
							OpaqueId:  BaseShare.Share.Id.OpaqueId,
						},
						Path: ".",
					},
					Destination: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./newname",
					},
				}
				res, err := s.Move(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				sharingCollaborationClient.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
			})

			It("renames a sharejail entry", func() {
				sharingCollaborationClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(nil, nil)

				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./oldname",
					},
					Destination: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./newname",
					},
				}
				res, err := s.Move(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				sharingCollaborationClient.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
			})

			It("refuses to move a file between shares", func() {
				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-shareddir-file",
					},
					Destination: &sprovider.Reference{
						Path: "/shares/share2-shareddir/share2-shareddir-file",
					},
				}
				res, err := s.Move(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("moves a file", func() {
				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
					Destination: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-shareddir-filenew",
					},
				}
				res, err := s.Move(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("ListFileVersions", func() {
			BeforeEach(func() {
				gatewayClient.On("ListFileVersions", mock.Anything, mock.Anything).Return(
					&sprovider.ListFileVersionsResponse{
						Status: status.NewOK(ctx),
						Versions: []*sprovider.FileVersion{
							{
								Size:  10,
								Mtime: 1,
								Etag:  "1",
								Key:   "1",
							},
							{
								Size:  20,
								Mtime: 2,
								Etag:  "2",
								Key:   "2",
							},
						},
					}, nil)
			})

			It("does not try to list versions of shares or the top-level dir", func() {
				req := &sprovider.ListFileVersionsRequest{
					Ref: &sprovider.Reference{
						Path: "/shares",
					},
				}
				res, err := s.ListFileVersions(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))

				req = &sprovider.ListFileVersionsRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/",
					},
				}
				res, err = s.ListFileVersions(ctx, req)
				gatewayClient.AssertNotCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("lists versions", func() {
				req := &sprovider.ListFileVersionsRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
				}
				res, err := s.ListFileVersions(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Versions)).To(Equal(2))
				version := res.Versions[0]
				Expect(version.Key).To(Equal("1"))
				Expect(version.Etag).To(Equal("1"))
				Expect(version.Mtime).To(Equal(uint64(1)))
				Expect(version.Size).To(Equal(uint64(10)))
			})
		})

		Describe("RestoreFileVersion", func() {
			BeforeEach(func() {
				gatewayClient.On("RestoreFileVersion", mock.Anything, mock.Anything).Return(
					&sprovider.RestoreFileVersionResponse{
						Status: status.NewOK(ctx),
					}, nil)
			})

			It("restores a file version", func() {
				req := &sprovider.RestoreFileVersionRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
					Key: "1",
				}
				res, err := s.RestoreFileVersion(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "RestoreFileVersion", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("InitiateFileUpload", func() {
			BeforeEach(func() {
				gatewayClient.On("InitiateFileUpload", mock.Anything, mock.Anything).Return(
					&gateway.InitiateFileUploadResponse{
						Status: status.NewOK(ctx),
						Protocols: []*gateway.FileUploadProtocol{
							{
								Opaque:         &types.Opaque{},
								Protocol:       "simple",
								UploadEndpoint: "https://localhost:9200/data",
								Token:          "thetoken",
							},
						},
					}, nil)
			})

			It("initiates a file upload", func() {
				req := &sprovider.InitiateFileUploadRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
				}
				res, err := s.InitiateFileUpload(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "InitiateFileUpload", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Protocols)).To(Equal(1))
				Expect(res.Protocols[0].Protocol).To(Equal("simple"))
				Expect(res.Protocols[0].UploadEndpoint).To(Equal("https://localhost:9200/data/thetoken"))
			})
		})

		Describe("SetArbitraryMetadata", func() {
			BeforeEach(func() {
				gatewayClient.On("SetArbitraryMetadata", mock.Anything, mock.Anything).Return(&sprovider.SetArbitraryMetadataResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("sets the metadata", func() {
				req := &sprovider.SetArbitraryMetadataRequest{
					Ref: &sprovider.Reference{
						ResourceId: ShareJail,
						Path:       "./share1-shareddir/share1-subdir/share1-subdir-file",
					},
					ArbitraryMetadata: &sprovider.ArbitraryMetadata{
						Metadata: map[string]string{
							"foo": "bar",
						},
					},
				}
				res, err := s.SetArbitraryMetadata(ctx, req)
				gatewayClient.AssertCalled(GinkgoT(), "SetArbitraryMetadata", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})
	})
})
