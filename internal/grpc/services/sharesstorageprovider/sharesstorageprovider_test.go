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
	"github.com/cs3org/reva/pkg/rgrpc/status"
	_ "github.com/cs3org/reva/pkg/share/manager/loader"
	"github.com/cs3org/reva/pkg/user"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Sharesstorageprovider", func() {
	var (
		config = map[string]interface{}{
			"mount_path":   "/shares",
			"gateway_addr": "127.0.0.1:1234",
			"driver":       "json",
			"drivers": map[string]map[string]interface{}{
				"json": {},
			},
		}
		ctx = user.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		})

		rootStatReq = &sprovider.StatRequest{
			Ref: &sprovider.Reference{
				Path: "/shares",
			},
		}
		rootListContainerReq = &sprovider.ListContainerRequest{
			Ref: &sprovider.Reference{
				Path: "/shares",
			},
		}

		s                    sprovider.ProviderAPIServer
		gw                   *mocks.GatewayClient
		sharesProviderClient *mocks.SharesProviderClient
	)

	BeforeEach(func() {
		sharesProviderClient = &mocks.SharesProviderClient{}
		gw = &mocks.GatewayClient{}
		gw.On("ListContainer", mock.Anything, &sprovider.ListContainerRequest{
			Ref: &sprovider.Reference{
				Path: "/share1-shareddir",
			},
		}).Return(
			&sprovider.ListContainerResponse{
				Status: status.NewOK(context.Background()),
				Infos: []*sprovider.ResourceInfo{
					&sprovider.ResourceInfo{
						Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path: "/share1-shareddir/share1-subdir",
						Id: &sprovider.ResourceId{
							StorageId: "share1-storageid",
							OpaqueId:  "subdir",
						},
						Size: 1,
					},
				},
			}, nil)

		gw.On("ListContainer", mock.Anything, &sprovider.ListContainerRequest{
			Ref: &sprovider.Reference{
				Path: "/share1-shareddir/share1-subdir",
			},
		}).Return(
			&sprovider.ListContainerResponse{
				Status: status.NewOK(context.Background()),
				Infos: []*sprovider.ResourceInfo{
					&sprovider.ResourceInfo{
						Type: sprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path: "/share1-shareddir/share1-subdir/share1-subdir-file",
						Id: &sprovider.ResourceId{
							StorageId: "share1-storageid",
							OpaqueId:  "file",
						},
						Size: 1,
					},
				},
			}, nil)

		gw.On("Stat", mock.Anything, mock.AnythingOfType("*providerv1beta1.StatRequest")).Return(
			func(_ context.Context, req *sprovider.StatRequest, _ ...grpc.CallOption) *sprovider.StatResponse {
				if req.Ref.GetPath() == "/share1-shareddir/share1-subdir" {
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
				} else if req.Ref.GetPath() == "/share1-shareddir/share1-subdir/share1-subdir-file" {
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
				} else {
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
							Size: 1234,
						},
					}
				}
			},
			nil)

	})

	JustBeforeEach(func() {
		p, err := provider.New("/shares", gw, sharesProviderClient)
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
					"file": tmpfile.Name(),
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
					&collaboration.ReceivedShare{
						State: collaboration.ShareState_SHARE_STATE_INVALID,
					},
					&collaboration.ReceivedShare{
						State: collaboration.ShareState_SHARE_STATE_PENDING,
					},
					&collaboration.ReceivedShare{
						State: collaboration.ShareState_SHARE_STATE_REJECTED,
					},
				},
			}, nil)
			res, err := s.ListContainer(ctx, rootListContainerReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(0))
		})
	})

	Context("with an accepted share", func() {
		BeforeEach(func() {
			sharesProviderClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					&collaboration.ReceivedShare{
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
					},
					&collaboration.ReceivedShare{
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
					},
				},
			}, nil)
		})

		Describe("Stat", func() {
			It("stats the root shares folder", func() {
				res, err := s.Stat(ctx, rootStatReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/shares"))
				Expect(res.Info.Size).To(Equal(uint64(1234)))
			})

			It("stats a shares folder", func() {
				statReq := &sprovider.StatRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir",
					},
				}
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/shares/share1-shareddir"))
				Expect(res.Info.Size).To(Equal(uint64(1234)))
			})

			It("merges permissions from multiple shares", func() {
				sharesProviderClient.On("ListReceivedShares", mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
					Status: status.NewOK(context.Background()),
					Shares: []*collaboration.ReceivedShare{
						&collaboration.ReceivedShare{
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
						},
						&collaboration.ReceivedShare{
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
						},
					},
				}, nil)
				statReq := &sprovider.StatRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir",
					},
				}
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/shares/share1-shareddir"))
				Expect(res.Info.PermissionSet.Stat).To(BeTrue())
				Expect(res.Info.PermissionSet.ListContainer).To(BeTrue())
			})

			It("stats a subfolder in a share", func() {
				statReq := &sprovider.StatRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-subdir",
					},
				}
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(res.Info.Path).To(Equal("/shares/share1-shareddir/share1-subdir"))
				Expect(res.Info.Size).To(Equal(uint64(10)))
			})

			It("stats a shared file", func() {
				statReq := &sprovider.StatRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-subdir/share1-subdir-file",
					},
				}
				res, err := s.Stat(ctx, statReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Info.Type).To(Equal(sprovider.ResourceType_RESOURCE_TYPE_FILE))
				Expect(res.Info.Path).To(Equal("/shares/share1-shareddir/share1-subdir/share1-subdir-file"))
				Expect(res.Info.Size).To(Equal(uint64(20)))
			})
		})

		Describe("ListContainer", func() {
			It("lists shares", func() {
				res, err := s.ListContainer(ctx, rootListContainerReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Infos)).To(Equal(1))

				entry := res.Infos[0]
				Expect(entry.Path).To(Equal("/shares/share1-shareddir"))
			})

			It("traverses into specific shares", func() {
				req := &sprovider.ListContainerRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir",
					},
				}
				res, err := s.ListContainer(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Infos)).To(Equal(1))

				entry := res.Infos[0]
				Expect(entry.Path).To(Equal("/shares/share1-shareddir/share1-subdir"))
			})

			It("traverses into subfolders of specific shares", func() {
				req := &sprovider.ListContainerRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-subdir",
					},
				}
				res, err := s.ListContainer(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(len(res.Infos)).To(Equal(1))

				entry := res.Infos[0]
				Expect(entry.Path).To(Equal("/shares/share1-shareddir/share1-subdir/share1-subdir-file"))
			})
		})

		Describe("InitiateFileDownload", func() {
			It("returns not found when not found", func() {
				gw.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found"),
				}, nil)

				req := &sprovider.InitiateFileDownloadRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/does-not-exist",
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
						&gateway.FileDownloadProtocol{
							Opaque:           &types.Opaque{},
							Protocol:         "simple",
							DownloadEndpoint: "https://localhost:9200/data",
							Token:            "thetoken",
						},
					},
				}, nil)
				req := &sprovider.InitiateFileDownloadRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-subdir/share1-subdir-file",
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
						Path: "/shares/share1-shareddir/share1-newsubdir",
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
				gw.On("Delete", mock.Anything, mock.Anything).Return(&sprovider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil)
			})

			It("rejects the share when deleting a share", func() {
				sharesProviderClient.On("UpdateReceivedShare", mock.Anything, mock.Anything).Return(nil, nil)
				req := &sprovider.DeleteRequest{
					Ref: &sprovider.Reference{
						Path: "/shares/share1-shareddir",
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
						Path: "/shares/share1-shareddir/share1-subdir/share1-subdir-file",
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

			It("refuses to move a share", func() {
				req := &sprovider.MoveRequest{
					Source: &sprovider.Reference{
						Path: "/shares/share1-shareddir",
					},
					Destination: &sprovider.Reference{
						Path: "/shares/newname",
					},
				}
				res, err := s.Move(ctx, req)
				gw.AssertNotCalled(GinkgoT(), "Move", mock.Anything, mock.Anything)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
				Expect(res.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
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
						Path: "/shares/share1-shareddir/share1-shareddir-file",
					},
					Destination: &sprovider.Reference{
						Path: "/shares/share1-shareddir/share1-shareddir-filenew",
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
						Path: "/shares/share1-shareddir/share1-shareddir-file",
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
						Path: "/shares/share1-shareddir/share1-shareddir-file",
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
						Path: "/shares/share1-shareddir/share1-shareddir-file",
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
						Path: "/shares/share1-shareddir/share1-subdir/share1-subdir-file",
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
