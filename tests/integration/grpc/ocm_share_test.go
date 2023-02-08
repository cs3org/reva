package grpc_test

import (
	"context"
	"io"
	"net/http"
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
				// err = webdavClient.Write(".", []byte("will-never-be-writter"), 0)
				// Expect(err).To(HaveOccurred())
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
			})
		})

		Context("einstein shares a folder with view permissions", func() {
			It("marie is able to see the content of the folder", func() {
				structure := helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{},
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
			})
		})

		Context("einstein shares a folder with editor permissions", func() {
			It("marie is able to see the content and upload resources", func() {
				structure := helpers.Folder{
					"foo": helpers.File{
						Content: "foo",
					},
					"dir": helpers.Folder{},
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
					"dir": helpers.Folder{},
					"new-file": helpers.File{
						Content: "new-file",
					},
				}))
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
