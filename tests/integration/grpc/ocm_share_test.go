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

package grpc_test

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	gatewaypb "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitev1beta1 "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmproviderpb "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/ocm/client"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/tests/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

var (
	editorPermissions = &provider.ResourcePermissions{
		InitiateFileDownload: true,
		InitiateFileUpload:   true,
		ListContainer:        true,
		GetPath:              true,
		Stat:                 true,
	}
	viewerPermissions = &provider.ResourcePermissions{
		Stat:                 true,
		InitiateFileDownload: true,
		GetPath:              true,
		ListContainer:        true,
	}
)

var _ = Describe("ocm share", func() {
	var (
		revads = map[string]*Revad{}

		variables = map[string]string{}

		ctxEinstein context.Context
		ctxMarie    context.Context
		cernboxgw   gatewaypb.GatewayAPIClient
		cesnetgw    gatewaypb.GatewayAPIClient
		cernbox     = &ocmproviderpb.ProviderInfo{
			Name:         "cernbox",
			FullName:     "CERNBox",
			Description:  "CERNBox provides cloud data storage to all CERN users.",
			Organization: "CERN",
			Domain:       "cernbox.cern.ch",
			Homepage:     "https://cernbox.web.cern.ch",
			Services: []*ocmproviderpb.Service{
				{
					Endpoint: &ocmproviderpb.ServiceEndpoint{
						Type: &ocmproviderpb.ServiceType{
							Name:        "OCM",
							Description: "CERNBox Open Cloud Mesh API",
						},
						Name:        "CERNBox - OCM API",
						Path:        "http://127.0.0.1:19001/ocm/",
						IsMonitored: true,
					},
					Host:       "127.0.0.1:19001",
					ApiVersion: "0.0.1",
				},
			},
		}
		einstein = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
				Idp:      "cernbox.cern.ch",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username:    "einstein",
			Mail:        "einstein@cern.ch",
			DisplayName: "Albert Einstein",
		}
		marie = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Idp:      "cesnet.cz",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username:    "marie",
			Mail:        "marie@cesnet.cz",
			DisplayName: "Marie Curie",
		}
	)

	JustBeforeEach(func() {
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		ctxEinstein = ctxWithAuthToken(tokenManager, einstein)
		ctxMarie = ctxWithAuthToken(tokenManager, marie)
		revads, err = startRevads(map[string]string{
			"cernboxgw":     "ocm-share/ocm-server-cernbox-grpc.toml",
			"cernboxwebdav": "ocm-share/cernbox-webdav-server.toml",
			"cernboxhttp":   "ocm-share/ocm-server-cernbox-http.toml",
			"cesnetgw":      "ocm-share/ocm-server-cesnet-grpc.toml",
			"cesnethttp":    "ocm-share/ocm-server-cesnet-http.toml",
		}, map[string]string{
			"providers": "ocm-providers.demo.json",
		}, map[string]Resource{
			"ocm_share_cernbox_file": File{Content: "{}"},
			"ocm_share_cesnet_file":  File{Content: "{}"},
			"invite_token_file":      File{Content: "{}"},
			"localhome_root":         Folder{},
		}, variables)
		Expect(err).ToNot(HaveOccurred())
		cernboxgw, err = pool.GetGatewayServiceClient(pool.Endpoint(revads["cernboxgw"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		cesnetgw, err = pool.GetGatewayServiceClient(pool.Endpoint(revads["cesnetgw"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		cernbox.Services[0].Endpoint.Path = "http://" + revads["cernboxhttp"].GrpcAddress + "/ocm"

		createHomeResp, err := cernboxgw.CreateHome(ctxEinstein, &provider.CreateHomeRequest{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createHomeResp.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
	})

	Describe("marie has already accepted the invitation workflow", func() {
		JustBeforeEach(func() {
			tknRes, err := cernboxgw.GenerateInviteToken(ctxEinstein, &invitev1beta1.GenerateInviteTokenRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(tknRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			_, err = client.New(&client.Config{}).InviteAccepted(ctxMarie, cernbox.Services[0].Endpoint.Path, &client.InviteAcceptedRequest{
				UserID:            marie.Id.OpaqueId,
				Email:             marie.Mail,
				RecipientProvider: "cernbox.cern.ch",
				Name:              marie.DisplayName,
				Token:             tknRes.InviteToken.Token,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Context("einstein shares a file with view permissions", func() {
			It("marie is able to see the content of the file", func() {
				fileToShare := &provider.Reference{
					Path: "/home/new-file",
				}
				By("creating a file")
				Expect(helpers.CreateFile(ctxEinstein, cernboxgw, fileToShare.Path, []byte("test"))).To(Succeed())

				By("share the file with marie")
				info, err := stat(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())

				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				By("marie access the share")
				listRes, err := cesnetgw.ListReceivedOCMShares(ctxMarie, &ocmv1beta1.ListReceivedOCMSharesRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(listRes.Shares).To(HaveLen(1))

				share := listRes.Shares[0]
				Expect(share.Protocols).To(HaveLen(1))

				protocol := share.Protocols[0]
				webdav, ok := protocol.Term.(*ocmv1beta1.Protocol_WebdavOptions)
				Expect(ok).To(BeTrue())

				webdavClient := gowebdav.NewClient(webdav.WebdavOptions.Uri, "", "")
				webdavClient.SetHeader("Authorization", "Bearer "+webdav.WebdavOptions.SharedSecret)
				d, err := webdavClient.Read(".")
				Expect(err).ToNot(HaveOccurred())
				Expect(d).To(Equal([]byte("test")))

				// TODO: enable once we don't send anymore the owner token
				// err = webdavClient.Write(".", []byte("will-never-be-written"), 0)
				// Expect(err).To(HaveOccurred())

				By("marie access the share using the ocm mount")
				ref := &provider.Reference{Path: ocmPath(share.Id, "")}
				statRes, err := cesnetgw.Stat(ctxMarie, &provider.StatRequest{Ref: ref})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				checkResourceInfo(statRes.Info, &provider.ResourceInfo{
					Id: &provider.ResourceId{
						StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
						OpaqueId:  share.Id.OpaqueId + ":/",
					},
					Name:          "new-file",
					Path:          ocmPath(share.Id, ""),
					Size:          4,
					Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
					PermissionSet: viewerPermissions,
				})

				data, err := helpers.Dowload(ctxMarie, cesnetgw, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal([]byte("test")))

				// TODO: enable once we don't send anymore the owner token
				// Expect(helpers.UploadGateway(ctxMarie, cesnetgw, ref, []byte("will-never-be-written"))).ToNot(Succeed())
			})
		})

		Context("einstein shares a file with editor permissions", func() {
			It("marie is able to modify the content of the file", func() {
				fileToShare := &provider.Reference{
					Path: "/home/new-file",
				}
				By("creating a file")
				Expect(helpers.CreateFile(ctxEinstein, cernboxgw, fileToShare.Path, []byte("test"))).To(Succeed())

				By("share the file with marie")
				info, err := stat(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())

				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				By("marie access the share and modify the content of the file")
				listRes, err := cesnetgw.ListReceivedOCMShares(ctxMarie, &ocmv1beta1.ListReceivedOCMSharesRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(listRes.Shares).To(HaveLen(1))

				share := listRes.Shares[0]
				Expect(share.Protocols).To(HaveLen(1))

				protocol := share.Protocols[0]
				webdav, ok := protocol.Term.(*ocmv1beta1.Protocol_WebdavOptions)
				Expect(ok).To(BeTrue())

				u := strings.TrimSuffix(webdav.WebdavOptions.Uri, "/new-file")
				webdavClient := gowebdav.NewClient(u, "", "")
				data := []byte("new-content")
				webdavClient.SetHeader("Authorization", "Bearer "+webdav.WebdavOptions.SharedSecret)
				webdavClient.SetHeader(ocdav.HeaderUploadLength, strconv.Itoa(len(data)))
				err = webdavClient.Write("new-file", data, 0)
				Expect(err).ToNot(HaveOccurred())

				By("check that the file was modified")
				newContent, err := download(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())
				Expect(newContent).To(Equal([]byte("new-content")))

				By("marie access the share using the ocm mount")
				ref := &provider.Reference{Path: ocmPath(share.Id, "")}
				statRes, err := cesnetgw.Stat(ctxMarie, &provider.StatRequest{Ref: ref})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				checkResourceInfo(statRes.Info, &provider.ResourceInfo{
					Id: &provider.ResourceId{
						StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
						OpaqueId:  share.Id.OpaqueId + ":/",
					},
					Name:          "new-file",
					Path:          ocmPath(share.Id, ""),
					Size:          uint64(len(data)),
					Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
					PermissionSet: editorPermissions,
				})

				data, err = helpers.Dowload(ctxMarie, cesnetgw, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal([]byte("new-content")))

				Expect(helpers.UploadGateway(ctxMarie, cesnetgw, ref, []byte("uploaded-from-ocm-mount"))).To(Succeed())
				newContent, err = download(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())
				Expect(newContent).To(Equal([]byte("uploaded-from-ocm-mount")))
			})
		})

		Context("einstein shares a folder with view permissions", func() {
			It("marie is able to see the content of the folder", func() {
				structure := helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{
						"foo": helpers.File{
							Content: "dir/foo",
						},
						"bar": helpers.Folder{},
					},
				}
				fileToShare := &provider.Reference{Path: "/home/ocm-share-folder"}
				Expect(helpers.CreateStructure(ctxEinstein, cernboxgw, fileToShare.Path, structure)).To(Succeed())

				By("share the file with marie")

				info, err := stat(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())

				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				By("marie see the content of the folder")
				listRes, err := cesnetgw.ListReceivedOCMShares(ctxMarie, &ocmv1beta1.ListReceivedOCMSharesRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(listRes.Shares).To(HaveLen(1))

				share := listRes.Shares[0]
				Expect(share.Protocols).To(HaveLen(1))

				protocol := share.Protocols[0]
				webdav, ok := protocol.Term.(*ocmv1beta1.Protocol_WebdavOptions)
				Expect(ok).To(BeTrue())

				webdavClient := gowebdav.NewClient(webdav.WebdavOptions.Uri, "", "")
				webdavClient.SetHeader("Authorization", "Bearer "+webdav.WebdavOptions.SharedSecret)

				ok, err = helpers.SameContentWebDAV(webdavClient, fileToShare.Path, structure)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())

				// By("check that marie does not have permissions to create files")
				// Expect(webdavClient.Write("new-file", []byte("new-file"), 0)).ToNot(Succeed())

				By("marie access the share using the ocm mount")
				ref := &provider.Reference{Path: ocmPath(share.Id, "dir")}
				listFolderRes, err := cesnetgw.ListContainer(ctxMarie, &provider.ListContainerRequest{
					Ref: ref,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(listFolderRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				checkResourceInfoList(listFolderRes.Infos, []*provider.ResourceInfo{
					{
						Id: &provider.ResourceId{
							StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
							OpaqueId:  share.Id.OpaqueId + ":/dir/foo",
						},
						Name:          "foo",
						Path:          ocmPath(share.Id, "dir/foo"),
						Size:          7,
						Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
						PermissionSet: viewerPermissions,
					},
					{
						Id: &provider.ResourceId{
							StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
							OpaqueId:  share.Id.OpaqueId + ":/dir/bar",
						},
						Name:          "bar",
						Path:          ocmPath(share.Id, "dir/bar"),
						Size:          0,
						Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						PermissionSet: viewerPermissions,
					},
				})

				// TODO: enable once we don't send anymore the owner token
				// newFile := &provider.Reference{Path: ocmPath(share.Id, "dir/new")}
				// Expect(helpers.UploadGateway(ctxMarie, cesnetgw, newFile, []byte("uploaded-from-ocm-mount"))).ToNot(Succeed())
			})
		})

		Context("einstein shares a folder with editor permissions", func() {
			It("marie is able to see the content and upload resources", func() {
				structure := helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{
						"foo": helpers.File{
							Content: "dir/foo",
						},
						"bar": helpers.Folder{},
					},
				}
				fileToShare := &provider.Reference{Path: "/home/ocm-share-folder"}

				Expect(helpers.CreateStructure(ctxEinstein, cernboxgw, fileToShare.Path, structure)).To(Succeed())

				By("share the file with marie")

				info, err := stat(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())

				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				By("marie can upload a file")
				listRes, err := cesnetgw.ListReceivedOCMShares(ctxMarie, &ocmv1beta1.ListReceivedOCMSharesRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(listRes.Shares).To(HaveLen(1))

				share := listRes.Shares[0]
				Expect(share.Protocols).To(HaveLen(1))

				protocol := share.Protocols[0]
				webdav, ok := protocol.Term.(*ocmv1beta1.Protocol_WebdavOptions)
				Expect(ok).To(BeTrue())

				webdavClient := gowebdav.NewClient(webdav.WebdavOptions.Uri, "", "")
				data := []byte("new-content")
				webdavClient.SetHeader("Authorization", "Bearer "+webdav.WebdavOptions.SharedSecret)
				webdavClient.SetHeader(ocdav.HeaderUploadLength, strconv.Itoa(len(data)))
				err = webdavClient.Write("new-file", data, 0)
				Expect(err).ToNot(HaveOccurred())

				Expect(webdavClient.Write("new-file", []byte("new-file"), 0)).To(Succeed())
				Expect(helpers.SameContentWebDAV(webdavClient, fileToShare.Path, helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{
						"foo": helpers.File{
							Content: "dir/foo",
						},
						"bar": helpers.Folder{},
					},
					"new-file": helpers.File{
						Content: "new-file",
					},
				}))

				By("marie access the share using the ocm mount")
				ref := &provider.Reference{Path: ocmPath(share.Id, "dir")}
				listFolderRes, err := cesnetgw.ListContainer(ctxMarie, &provider.ListContainerRequest{
					Ref: ref,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(listFolderRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				checkResourceInfoList(listFolderRes.Infos, []*provider.ResourceInfo{
					{
						Id: &provider.ResourceId{
							StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
							OpaqueId:  share.Id.OpaqueId + ":/dir/foo",
						},
						Name:          "foo",
						Path:          ocmPath(share.Id, "dir/foo"),
						Size:          7,
						Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
						PermissionSet: editorPermissions,
					},
					{
						Id: &provider.ResourceId{
							StorageId: "984e7351-2729-4417-99b4-ab5e6d41fa97",
							OpaqueId:  share.Id.OpaqueId + ":/dir/bar",
						},
						Name:          "bar",
						Path:          ocmPath(share.Id, "dir/bar"),
						Size:          0,
						Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						PermissionSet: editorPermissions,
					},
				})

				// create a new file
				newFile := &provider.Reference{Path: ocmPath(share.Id, "dir/new-file")}
				Expect(helpers.UploadGateway(ctxMarie, cesnetgw, newFile, []byte("uploaded-from-ocm-mount"))).To(Succeed())
				Expect(helpers.SameContentWebDAV(webdavClient, fileToShare.Path, helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{
						"foo": helpers.File{
							Content: "dir/foo",
						},
						"bar": helpers.Folder{},
						"new-file": helpers.File{
							Content: "uploaded-from-ocm-mount",
						},
					},
					"new-file": helpers.File{
						Content: "new-file",
					},
				}))

				// create a new directory
				newDir := &provider.Reference{Path: ocmPath(share.Id, "dir/new-dir")}
				createDirRes, err := cesnetgw.CreateContainer(ctxMarie, &provider.CreateContainerRequest{
					Ref: newDir,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createDirRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(helpers.SameContentWebDAV(webdavClient, fileToShare.Path, helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{
						"foo": helpers.File{
							Content: "dir/foo",
						},
						"bar": helpers.Folder{},
						"new-file": helpers.File{
							Content: "uploaded-from-ocm-mount",
						},
						"new-dir": helpers.Folder{},
					},
					"new-file": helpers.File{
						Content: "new-file",
					},
				}))
			})
		})

		Context("einstein creates twice the share to marie", func() {
			It("fail with already existing error", func() {
				fileToShare := &provider.Reference{Path: "/home/double-share"}
				Expect(helpers.CreateFolder(ctxEinstein, cernboxgw, fileToShare.Path)).To(Succeed())

				By("share the file with marie")

				info, err := stat(ctxEinstein, cernboxgw, fileToShare)
				Expect(err).ToNot(HaveOccurred())

				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				By("resharing the same file with marie")

				createShareRes2, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: info.Id,
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes2.Status.Code).To(Equal(rpcv1beta1.Code_CODE_ALREADY_EXISTS))
			})
		})

		Context("einstein creates a share on a not existing resource", func() {
			It("fail with not found error", func() {
				cesnet, err := cernboxgw.GetInfoByDomain(ctxEinstein, &ocmproviderpb.GetInfoByDomainRequest{
					Domain: "cesnet.cz",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(cesnet.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				createShareRes, err := cernboxgw.CreateOCMShare(ctxEinstein, &ocmv1beta1.CreateOCMShareRequest{
					ResourceId: &provider.ResourceId{StorageId: "123e4567-e89b-12d3-a456-426655440000", OpaqueId: "NON_EXISTING_FILE"},
					Grantee: &provider.Grantee{
						Id: &provider.Grantee_UserId{
							UserId: marie.Id,
						},
					},
					AccessMethods: []*ocmv1beta1.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
					RecipientMeshProvider: cesnet.ProviderInfo,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createShareRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
			})
		})

	})
})

func stat(ctx context.Context, gw gatewaypb.GatewayAPIClient, ref *provider.Reference) (*provider.ResourceInfo, error) {
	statRes, err := gw.Stat(ctx, &provider.StatRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(statRes.Status.Message)
	}
	return statRes.Info, nil
}

func download(ctx context.Context, gw gatewaypb.GatewayAPIClient, ref *provider.Reference) ([]byte, error) {
	initRes, err := gw.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{Ref: ref})
	if err != nil {
		return nil, err
	}

	var token, endpoint string
	for _, p := range initRes.Protocols {
		if p.Protocol == "simple" {
			token, endpoint = p.Token, p.DownloadEndpoint
		}
	}
	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	return io.ReadAll(httpRes.Body)
}

func ocmPath(id *ocmv1beta1.ShareId, p string) string {
	return filepath.Join("/ocm", id.OpaqueId, p)
}

func checkResourceInfo(info, target *provider.ResourceInfo) {
	Expect(info.Id).To(Equal(target.Id))
	Expect(info.Name).To(Equal(target.Name))
	Expect(info.Path).To(Equal(target.Path))
	Expect(info.Size).To(Equal(target.Size))
	Expect(info.Type).To(Equal(target.Type))
	Expect(info.PermissionSet).To(Equal(target.PermissionSet))
}

func mapResourceInfos(l []*provider.ResourceInfo) map[string]*provider.ResourceInfo {
	m := make(map[string]*provider.ResourceInfo)
	for _, e := range l {
		m[e.Path] = e
	}
	return m
}

func checkResourceInfoList(l1, l2 []*provider.ResourceInfo) {
	m1, m2 := mapResourceInfos(l1), mapResourceInfos(l2)
	Expect(l1).To(HaveLen(len(l2)))

	for k, ri1 := range m1 {
		ri2, ok := m2[k]
		Expect(ok).To(BeTrue())
		checkResourceInfo(ri1, ri2)
	}
}
