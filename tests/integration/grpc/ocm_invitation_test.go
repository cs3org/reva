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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

type generateInviteResponse struct {
	Token       string `json:"token"`
	Description string `json:"descriptions"`
	Expiration  uint64 `json:"expiration"`
	InviteLink  string `json:"invite_link"`
}

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
		err    error
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
		revads, err = startRevads(map[string]string{
			"cernboxgw":   "ocm-server-cernbox-grpc.toml",
			"cernboxhttp": "ocm-server-cernbox-http.toml",
			"cesnetgw":    "ocm-server-cesnet-grpc.toml",
			"cesnethttp":  "ocm-server-cesnet-http.toml",
		}, map[string]string{
			"providers": "ocm-providers.demo.json",
		}, nil, variables)
		Expect(err).ToNot(HaveOccurred())
		cernboxgw, err = pool.GetGatewayServiceClient(pool.Endpoint(revads["cernboxgw"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		cesnetgw, err = pool.GetGatewayServiceClient(pool.Endpoint(revads["cesnetgw"].GrpcAddress))
		Expect(err).ToNot(HaveOccurred())
		cernbox.Services[0].Endpoint.Path = "http://" + revads["cernboxhttp"].GrpcAddress + "/ocm"
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
		Expect(os.RemoveAll(inviteTokenFile)).To(Succeed())
	})

	Describe("einstein and marie do not know each other", func() {
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJSONFile(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		Context("einstein generates a token", func() {
			It("will complete the workflow ", func() {
				invitationTknRes, err := cernboxgw.GenerateInviteToken(ctxEinstein, &invitepb.GenerateInviteTokenRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(invitationTknRes.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(invitationTknRes.InviteToken).ToNot(BeNil())
				forwardRes, err := cesnetgw.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
					OriginSystemProvider: cernbox,
					InviteToken:          invitationTknRes.InviteToken,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_OK))

				Expect(forwardRes.DisplayName).To(Equal(einstein.DisplayName))
				Expect(forwardRes.Email).To(Equal(einstein.Mail))
				Expect(forwardRes.UserId).To(Equal(einstein.Id))

				usersRes1, err := cernboxgw.FindAcceptedUsers(ctxEinstein, &invitepb.FindAcceptedUsersRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(usersRes1.Status.Code).To(Equal(rpc.Code_CODE_OK))
				Expect(usersRes1.AcceptedUsers).To(HaveLen(1))
				info1 := usersRes1.AcceptedUsers[0]
				Expect(ocmUserEqual(info1, marie)).To(BeTrue())

				usersRes2, err := cesnetgw.FindAcceptedUsers(ctxMarie, &invitepb.FindAcceptedUsersRequest{})
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
			inviteTokenFile, err = helpers.TempJSONFile(map[string]map[string][]*userpb.User{
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

		Context("marie accepts a new invite token generated by einstein", func() {
			It("fails with already exists code", func() {
				inviteTknRes, err := cernboxgw.GenerateInviteToken(ctxEinstein, &invitepb.GenerateInviteTokenRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(inviteTknRes.Status.Code).To(Equal(rpc.Code_CODE_OK))

				forwardRes, err := cesnetgw.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
					InviteToken:          inviteTknRes.InviteToken,
					OriginSystemProvider: cernbox,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_ALREADY_EXISTS))
			})
		})
	})

	Describe("marie accepts an expired token", func() {
		expiredToken := &invitepb.InviteToken{
			Token:  "token",
			UserId: einstein.Id,
			Expiration: &typesv1beta1.Timestamp{
				Seconds: 0,
			},
			Description: "expired token",
		}
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJSONFile(map[string]map[string]*invitepb.InviteToken{
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
			forwardRes, err := cesnetgw.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
				InviteToken:          expiredToken,
				OriginSystemProvider: cernbox,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
		})
	})

	Describe("marie accept a not existing token", func() {
		BeforeEach(func() {
			inviteTokenFile, err = helpers.TempJSONFile(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			variables = map[string]string{
				"invite_token_file": inviteTokenFile,
			}
		})

		It("will not complete the invitation workflow", func() {
			forwardRes, err := cesnetgw.ForwardInvite(ctxMarie, &invitepb.ForwardInviteRequest{
				InviteToken: &invitepb.InviteToken{
					Token: "not-existing-token",
				},
				OriginSystemProvider: cernbox,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(forwardRes.Status.Code).To(Equal(rpc.Code_CODE_NOT_FOUND))
		})
	})

	Context("clients use the http endpoints exposed by sciencemesh", func() {
		var (
			cesnetURL             string
			cernboxURL            string
			tknMarie, tknEinstein string
			token                 string
		)

		JustBeforeEach(func() {
			cesnetURL = revads["cesnethttp"].GrpcAddress
			cernboxURL = revads["cernboxhttp"].GrpcAddress

			var ok bool
			tknMarie, ok = ctxpkg.ContextGetToken(ctxMarie)
			Expect(ok).To(BeTrue())
			tknEinstein, ok = ctxpkg.ContextGetToken(ctxEinstein)
			Expect(ok).To(BeTrue())

			tknRes, err := cernboxgw.GenerateInviteToken(ctxEinstein, &invitepb.GenerateInviteTokenRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(tknRes.Status.Code).To(Equal(rpc.Code_CODE_OK))
			token = tknRes.InviteToken.Token
		})

		acceptInvite := func(revaToken, domain, provider, token string) int {
			d, err := json.Marshal(map[string]string{
				"token":          token,
				"providerDomain": provider,
			})
			Expect(err).ToNot(HaveOccurred())
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, fmt.Sprintf("http://%s/sciencemesh/accept-invite", domain), bytes.NewReader(d))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("x-access-token", revaToken)
			req.Header.Set("content-type", "application/json")

			res, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer res.Body.Close()

			return res.StatusCode
		}

		findAccepted := func(revaToken, domain string) ([]*userpb.User, int) {
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, fmt.Sprintf("http://%s/sciencemesh/find-accepted-users", domain), nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("x-access-token", revaToken)

			res, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer res.Body.Close()

			var users []*userpb.User
			_ = json.NewDecoder(res.Body).Decode(&users)
			return users, res.StatusCode
		}

		generateToken := func(revaToken, domain string) (*generateInviteResponse, int) {
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, fmt.Sprintf("http://%s/sciencemesh/generate-invite", domain), nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("x-access-token", revaToken)

			res, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer res.Body.Close()

			var inviteRes generateInviteResponse
			Expect(json.NewDecoder(res.Body).Decode(&inviteRes)).To(Succeed())
			return &inviteRes, res.StatusCode
		}

		Context("einstein and marie do not know each other", func() {

			Context("marie is not logged-in", func() {
				It("fails with permission denied", func() {
					code := acceptInvite("", cesnetURL, "cernbox.cern.ch", token)
					Expect(code).To(Equal(http.StatusUnauthorized))
				})
			})
			It("complete the invitation workflow", func() {
				users, code := findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{})).To(BeTrue())

				code = acceptInvite(tknMarie, cesnetURL, "cernbox.cern.ch", token)
				Expect(code).To(Equal(http.StatusOK))

				users, code = findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{marie})).To(BeTrue())
			})
		})

		Context("marie already accepted an invitation before", func() {
			BeforeEach(func() {
				inviteTokenFile, err = helpers.TempJSONFile(map[string]map[string][]*userpb.User{
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

			It("fails the invitation workflow", func() {
				users, code := findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{marie})).To(BeTrue())

				code = acceptInvite(tknMarie, cesnetURL, "cernbox.cern.ch", token)
				Expect(code).To(Equal(http.StatusConflict))

				users, code = findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{marie})).To(BeTrue())
			})
		})

		Context("marie uses an expired token", func() {
			expiredToken := &invitepb.InviteToken{
				Token:  "token",
				UserId: einstein.Id,
				Expiration: &typesv1beta1.Timestamp{
					Seconds: 0,
				},
				Description: "expired token",
			}
			BeforeEach(func() {
				inviteTokenFile, err = helpers.TempJSONFile(map[string]map[string]*invitepb.InviteToken{
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
				users, code := findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{})).To(BeTrue())

				code = acceptInvite(tknMarie, cesnetURL, "cernbox.cern.ch", expiredToken.Token)
				Expect(code).To(Equal(http.StatusBadRequest))

				users, code = findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{})).To(BeTrue())
			})
		})

		Context("generate the token from http apis", func() {
			BeforeEach(func() {
				inviteTokenFile, err = helpers.TempJSONFile(map[string]map[string]*invitepb.InviteToken{})
				Expect(err).ToNot(HaveOccurred())
				variables = map[string]string{
					"invite_token_file": inviteTokenFile,
				}
			})
			It("succeeds", func() {
				users, code := findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{})).To(BeTrue())

				ocmToken, code := generateToken(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))

				code = acceptInvite(tknMarie, cesnetURL, "cernbox.cern.ch", ocmToken.Token)
				Expect(code).To(Equal(http.StatusOK))

				users, code = findAccepted(tknEinstein, cernboxURL)
				Expect(code).To(Equal(http.StatusOK))
				Expect(ocmUsersEqual(users, []*userpb.User{marie})).To(BeTrue())
			})
		})

	})
})

func ocmUsersEqual(u1, u2 []*userpb.User) bool {
	if len(u1) != len(u2) {
		return false
	}
	for i := range u1 {
		if !ocmUserEqual(u1[i], u2[i]) {
			return false
		}
	}
	return true
}
