package grpc_test

import (
	"context"
	"fmt"
	"os"

	gatewaypb "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmproviderpb "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/tests/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/metadata"
)

func ctxWithAuthToken(tokenManager token.Manager, user *userpb.User) context.Context {
	ctx := context.Background()
	scope, err := scope.AddOwnerScope(nil)
	Expect(err).ToNot(HaveOccurred())
	tkn, err := tokenManager.MintToken(ctx, user, scope)
	Expect(err).ToNot(HaveOccurred())
	ctx = ctxpkg.ContextSetToken(ctx, tkn)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, tkn)
	ctx = ctxpkg.ContextSetUser(ctx, user)
	return ctx
}

func ocmUserEqual(u1, u2 *userpb.User) bool {
	return utils.UserEqual(u1.Id, u2.Id) && u1.DisplayName == u2.DisplayName && u1.Mail == u2.Mail
}

var _ = Describe("ocm invitation workflow", func() {
	var (
		err     error
		revads1 = map[string]*Revad{}
		revads2 = map[string]*Revad{}

		variables = map[string]string{}

		ctxEinstein context.Context
		ctxMarie    context.Context
		gateway1    gatewaypb.GatewayAPIClient
		gateway2    gatewaypb.GatewayAPIClient
		provider1   = &ocmproviderpb.ProviderInfo{
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
		inviteTokenFile string
		einstein        = &userpb.User{
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
		revads1, err = startRevads(map[string]string{
			"gateway": "ocm-server-grpc.toml",
			"http":    "ocm-server-http.toml",
		}, variables)
		Expect(err).ToNot(HaveOccurred())
		revads2, err = startRevads(map[string]string{
			"gateway": "ocm-server-grpc.toml",
			"http":    "ocm-server-http.toml",
		}, variables)
		Expect(err).ToNot(HaveOccurred())
		gateway1, err = pool.GetGatewayServiceClient(pool.Endpoint(revads1["gateway"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		gateway2, err = pool.GetGatewayServiceClient(pool.Endpoint(revads2["gateway"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		provider1.Services[0].Endpoint.Path = "http://" + revads1["http"].GrpcAddress + "/ocm"
	})

	AfterEach(func() {
		for _, r := range revads1 {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
		for _, r := range revads2 {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
		Expect(os.RemoveAll(inviteTokenFile)).To(Succeed())
	})

	Describe("einstein and marie do not know each other", func() {
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJsonFile(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		Context("einstein generates a token", func() {
			It("will complete the workflow ", func() {
				invitationTknRes, err := gateway1.GenerateInviteToken(ctxEinstein, &invitepb.GenerateInviteTokenRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(invitationTknRes.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(invitationTknRes.InviteToken).ToNot(BeNil())

				forwardRes, err := gateway2.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
					OriginSystemProvider: provider1,
					InviteToken:          invitationTknRes.InviteToken,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_OK))

				Expect(forwardRes.DisplayName).To(Equal(einstein.DisplayName))
				Expect(forwardRes.Email).To(Equal(einstein.Mail))
				Expect(forwardRes.UserId).To(Equal(einstein.Id))

				usersRes1, err := gateway1.FindAcceptedUsers(ctxEinstein, &invitepb.FindAcceptedUsersRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(usersRes1.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(usersRes1.AcceptedUsers).To(HaveLen(1))
				info1 := usersRes1.AcceptedUsers[0]
				Expect(ocmUserEqual(info1, marie)).To(BeTrue())

				usersRes2, err := gateway2.FindAcceptedUsers(ctxMarie, &invitepb.FindAcceptedUsersRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(usersRes2.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(usersRes2.AcceptedUsers).To(HaveLen(1))
				info2 := usersRes2.AcceptedUsers[0]
				Expect(ocmUserEqual(info2, einstein)).To(BeTrue())
			})

		})
	})

	Describe("an invitation workflow has been already completed between einstein and marie", func() {
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJsonFile(map[string]map[string][]*userpb.User{
				"accepted_users": {
					einstein.Id.OpaqueId: {marie},
					marie.Id.OpaqueId:    {einstein},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		Context("marie accept a new invite token generated by einstein", func() {
			It("fails with already exists code", func() {
				inviteTknRes, err := gateway1.GenerateInviteToken(ctxEinstein, &invitepb.GenerateInviteTokenRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(inviteTknRes.Status.Code).To(Equal(rpc.Code_CODE_OK))

				forwardRes, err := gateway2.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
					InviteToken:          inviteTknRes.InviteToken,
					OriginSystemProvider: provider1,
				})
				Expect(err).ToNot(HaveOccurred())
				fmt.Println(forwardRes.Status)
				Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_ALREADY_EXISTS))
			})
		})
	})

	Describe("marie accept an expired token", func() {
		expiredToken := &invitepb.InviteToken{
			Token:  "token",
			UserId: einstein.Id,
			Expiration: &typesv1beta1.Timestamp{
				Seconds: 0,
			},
			Description: "expired token",
		}
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJsonFile(map[string]map[string]*invitepb.InviteToken{
				"invites": {
					expiredToken.Token: expiredToken,
				},
			})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		It("will not complete the invitation workflow", func() {
			forwardRes, err := gateway2.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
				InviteToken:          expiredToken,
				OriginSystemProvider: provider1,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
		})
	})

	Describe("marie accept a not existing token", func() {
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJsonFile(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		It("will not complete the invitation workflow", func() {
			forwardRes, err := gateway2.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
				InviteToken: &invitepb.InviteToken{
					Token: "not-existing-token",
				},
				OriginSystemProvider: provider1,
			})
			Expect(err).ToNot(HaveOccurred())
			fmt.Println(forwardRes.Status)
			Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_NOT_FOUND))
		})
	})
})
