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
	"io/ioutil"
	"os"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	sprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	provider "github.com/cs3org/reva/internal/grpc/services/sharesstorageprovider"
	mocks "github.com/cs3org/reva/internal/grpc/services/sharesstorageprovider/mocks"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	_ "github.com/cs3org/reva/pkg/share/manager/loader"
	"github.com/cs3org/reva/pkg/utils"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var (
	BaseShare = &collaboration.ReceivedShare{
		State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
		Share: &collaboration.Share{
			ResourceId: &sprovider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  "shareddir",
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &sprovider.ResourcePermissions{
					Stat:          true,
					ListContainer: true,
				},
			},
		},
	}

	BaseShareTwo = &collaboration.ReceivedShare{
		State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
		Share: &collaboration.Share{
			ResourceId: &sprovider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  "shareddir",
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &sprovider.ResourcePermissions{
					Stat:          true,
					ListContainer: true,
				},
			},
		},
	}

	BaseStatRequest = &sprovider.StatRequest{
		Ref: &sprovider.Reference{
			ResourceId: &sprovider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "shareddir",
			},
			Path: ".",
		},
	}

	BaseListContainerRequest = &sprovider.ListContainerRequest{
		Ref: &sprovider.Reference{
			ResourceId: &sprovider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "shareddir",
			},
			Path: ".",
		},
	}
)

var _ = Describe("Sharesstorageprovider", func() {
	var (
		config = map[string]interface{}{
			"gateway_addr": "127.0.0.1:1234",
			"driver":       "json",
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

		s                    sprovider.ProviderAPIServer
		gw                   *mocks.GatewayClient
		sharesProviderClient *mocks.SharesProviderClient
	)

	BeforeEach(func() {
		sharesProviderClient = &mocks.SharesProviderClient{}

		gw = &mocks.GatewayClient{}

		// mock stat requests
		gw.On("Stat", mock.Anything, mock.AnythingOfType("*providerv1beta1.StatRequest")).Return(
			func(_ context.Context, req *sprovider.StatRequest, _ ...grpc.CallOption) *sprovider.StatResponse {
				switch req.Ref.GetPath() {
				case "./share1-shareddir/share1-subdir":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "/share1-shareddir/share1-subdir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								OpaqueId:  "subdir",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 10,
						},
					}
				case "./share1-shareddir/share1-subdir/share1-subdir-file":
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_FILE,
							Path: "/share1-shareddir/share1-subdir/share1-subdir-file",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								OpaqueId:  "file",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 20,
						},
					}
				default:
					return &sprovider.StatResponse{
						Status: status.NewOK(context.Background()),
						Info: &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "/share1-shareddir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								OpaqueId:  "shareddir",
							},
							PermissionSet: &sprovider.ResourcePermissions{
								Stat: true,
							},
							Size: 100,
						},
					}
				}
			},
			nil)

		gw.On("ListContainer", mock.Anything, mock.AnythingOfType("*providerv1beta1.ListContainerRequest")).Return(
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
							Path: "/share1-shareddir/share1-subdir",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
								OpaqueId:  "subdir",
							},
							Size: 1,
						})
					case "./share1-subdir":
						resp.Infos = append(resp.Infos, &sprovider.ResourceInfo{
							Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
							Path: "/share1-shareddir/share1-subdir/share1-subdir-file",
							Id: &sprovider.ResourceId{
								StorageId: "share1-storageid",
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
		p, err := provider.New(gw, sharesProviderClient)
		Expect(err).ToNot(HaveOccurred())
		s = p.(sprovider.ProviderAPIServer)
		Expect(s).ToNot(BeNil())
	})

	Describe("NewDefault", func() {
		It("returns a new service instance", func() {
			tmpfile, err := ioutil.TempFile("", "eos-unit-test-shares-*.json")
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
		It("only considers accepted shares", func() {
			sharesProviderClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					{
						Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
						State: collaboration.ShareState_SHARE_STATE_INVALID,
					},
					{
						Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
						State: collaboration.ShareState_SHARE_STATE_PENDING,
					},
					{
						Share: &collaboration.Share{ResourceId: &sprovider.ResourceId{}},
						State: collaboration.ShareState_SHARE_STATE_REJECTED,
					},
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
			sharesProviderClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{BaseShare, BaseShareTwo},
			}, nil)
		})

		Describe("Stat", func() {
			It("stats the root shares folder", func() {
				res, err := s.Stat(ctx, BaseStatRequest)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/share1-shareddir"))
				// Expect(res.Info.Size).To(Equal(uint64(300))) TODO: Why 300?
				Expect(res.Info.Size).To(Equal(uint64(100)))
			})

			It("stats a shares folder", func() {
				statReq := BaseStatRequest
				statReq.Ref.ResourceId.OpaqueId = "shareddir"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/share1-shareddir"))
				Expect(res.Info.Size).To(Equal(uint64(100)))
			})

			It("merges permissions from multiple shares", func() {
				sharesProviderClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{
						{
							State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
							Share: &collaboration.Share{
								ResourceId: &sprovider.ResourceId{
									StorageId: "share1-storageid",
									OpaqueId:  "shareddir",
								},
								Permissions: &collaboration.SharePermissions{
									Permissions: &sprovider.ResourcePermissions{
										Stat: true,
									},
								},
							},
							MountPoint: &sprovider.Reference{Path: "share1-shareddir"},
						},
						{
							State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
							Share: &collaboration.Share{
								ResourceId: &sprovider.ResourceId{
									StorageId: "share1-storageid",
									OpaqueId:  "shareddir",
								},
								Permissions: &collaboration.SharePermissions{
									Permissions: &sprovider.ResourcePermissions{
										ListContainer: true,
									},
								},
							},
							MountPoint: &sprovider.Reference{Path: "share2-shareddir"},
						},
					},
				}, nil)
				statReq := BaseStatRequest
				statReq.Ref.ResourceId.OpaqueId = "shareddir"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/share1-shareddir"))
				Expect(res.Info.PermissionSet.Stat).To(BeTrue())
				// Expect(res.Info.PermissionSet.ListContainer).To(BeTrue()) // TODO reenable
			})

			It("stats a subfolder in a share", func() {
				statReq := BaseStatRequest
				statReq.Ref.Path = "./share1-shareddir/share1-subdir"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/share1-shareddir/share1-subdir"))
				Expect(res.Info.Size).To(Equal(uint64(10)))
			})

			It("stats a shared file", func() {
				statReq := BaseStatRequest
				statReq.Ref.Path = "./share1-shareddir/share1-subdir/share1-subdir-file"
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(res.Info).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_FILE))
				Expect(res.Info.Path).To(Equal("/share1-shareddir/share1-subdir/share1-subdir-file"))
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
				Expect(entry.Path).To(Equal("/share1-shareddir/share1-subdir"))
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
				Expect(entry.Path).To(Equal("/share1-shareddir/share1-subdir/share1-subdir-file"))
			})
		})

		Describe("InitiateFileDownload", func() {
			It("returns not found when not found", func() {
				gw.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found"),
				}, nil)

				req := &sprovider.InitiateFileDownloadRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/does-not-exist",
					},
				}
				res, err := s.InitiateFileDownload(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_NOT_FOUND))
			})

			It("initiates the download of an existing file", func() {
				gw.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
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
						ResourceId: BaseShare.Share.ResourceId,
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
				gw.On("CreateContainer", mock.Anything, mock.Anything).Return(&sprovider.CreateContainerResponse{
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
				gw.AssertNotCalled(GinkgoT(), "CreateContainer", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("creates a directory", func() {
				req := &sprovider.CreateContainerRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-newsubdir",
					},
				}
				res, err := s.CreateContainer(ctx, req)
				gw.AssertCalled(GinkgoT(), "CreateContainer", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})

		Describe("Delete", func() {
			BeforeEach(func() {
				gw.On("Delete", mock.Anything, mock.Anything).Return(
					&sprovider.DeleteResponse{Status: status.NewOK(ctx)}, nil)
			})

			It("rejects the share when deleting a share", func() {
				sharesProviderClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(
					&collaboration.UpdateReceivedShareResponse{Status: status.NewOK(ctx)}, nil)
				req := &sprovider.DeleteRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       ".",
					},
				}
				res, err := s.Delete(ctx, req)
				gw.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))

				sharesProviderClient.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
			})

			It("deletes a file", func() {
				req := &sprovider.DeleteRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-subdir/share1-subdir-file",
					},
				}
				res, err := s.Delete(ctx, req)
				gw.AssertCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("Move", func() {
			BeforeEach(func() {
				gw.On("Move", mock.Anything, mock.Anything).Return(&sprovider.MoveResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("renames a share", func() {
				sharesProviderClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(nil, nil)

				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       ".",
					},
					Destination: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./newname",
					},
				}
				res, err := s.Move(ctx, req)
				gw.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				sharesProviderClient.AssertCalled(GinkgoT(), "UpdateReceivedShare", mock.Anything, mock.Anything)
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
				gw.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("moves a file", func() {
				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
					Destination: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-shareddir-filenew",
					},
				}
				res, err := s.Move(ctx, req)
				gw.AssertCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("ListFileVersions", func() {
			BeforeEach(func() {
				gw.On("ListFileVersions", mock.Anything, mock.Anything).Return(
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
				gw.AssertNotCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))

				req = &sprovider.ListFileVersionsRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/",
					},
				}
				res, err = s.ListFileVersions(ctx, req)
				gw.AssertNotCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			})

			It("lists versions", func() {
				req := &sprovider.ListFileVersionsRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
				}
				res, err := s.ListFileVersions(ctx, req)
				gw.AssertCalled(GinkgoT(), "ListFileVersions", mock.Anything, mock.Anything)
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
				gw.On("RestoreFileVersion", mock.Anything, mock.Anything).Return(
					&sprovider.RestoreFileVersionResponse{
						Status: status.NewOK(ctx),
					}, nil)
			})

			It("restores a file version", func() {
				req := &sprovider.RestoreFileVersionRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
					Key: "1",
				}
				res, err := s.RestoreFileVersion(ctx, req)
				gw.AssertCalled(GinkgoT(), "RestoreFileVersion", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})

		Describe("InitiateFileUpload", func() {
			BeforeEach(func() {
				gw.On("InitiateFileUpload", mock.Anything, mock.Anything).Return(
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
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-shareddir-file",
					},
				}
				res, err := s.InitiateFileUpload(ctx, req)
				gw.AssertCalled(GinkgoT(), "InitiateFileUpload", mock.Anything, mock.Anything)
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
				gw.On("SetArbitraryMetadata", mock.Anything, mock.Anything).Return(&sprovider.SetArbitraryMetadataResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("sets the metadata", func() {
				req := &sprovider.SetArbitraryMetadataRequest{
					Ref: &sprovider.Reference{
						ResourceId: BaseShare.Share.ResourceId,
						Path:       "./share1-shareddir/share1-subdir/share1-subdir-file",
					},
					ArbitraryMetadata: &sprovider.ArbitraryMetadata{
						Metadata: map[string]string{
							"foo": "bar",
						},
					},
				}
				res, err := s.SetArbitraryMetadata(ctx, req)
				gw.AssertCalled(GinkgoT(), "SetArbitraryMetadata", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
			})
		})
	})
})
