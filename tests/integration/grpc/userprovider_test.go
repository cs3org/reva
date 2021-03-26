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

package grpc_test

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user providers", func() {
	var (
		dependencies map[string]string
		revads       map[string]*Revad

		existingIdp string

		ctx           context.Context
		serviceClient userpb.UserAPIClient
	)

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		// Add auth token
		user := &userpb.User{
			Id: &userpb.UserId{
				Idp:      existingIdp,
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
		}
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user)
		Expect(err).ToNot(HaveOccurred())
		ctx = token.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, t)
		ctx = ruser.ContextSetUser(ctx, user)

		revads, err = startRevads(dependencies, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		serviceClient, err = pool.GetUserProviderServiceClient(revads["users"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed))
		}
	})

	var assertGetUserByClaimResponses = func() {
		It("gets users as expected", func() {
			tests := map[string]string{
				"mail":     "einstein@example.org",
				"username": "einstein",
				"uid":      "123",
			}

			for claim, value := range tests {
				user, err := serviceClient.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: claim, Value: value})
				Expect(err).ToNot(HaveOccurred())
				Expect(user.User.Mail).To(Equal("einstein@example.org"))
			}
		})
	}

	var assertGetUserResponses = func() {
		It("gets users as expected", func() {
			tests := []struct {
				name   string
				userID *userpb.UserId
				want   *userpb.GetUserResponse
			}{
				{
					name: "simple",
					userID: &userpb.UserId{
						Idp:      existingIdp,
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					},
					want: &userpb.GetUserResponse{
						Status: &rpc.Status{
							Code: 1,
						},
						User: &userpb.User{
							Username:    "marie",
							Mail:        "marie@example.org",
							DisplayName: "Marie Curie",
							Groups: []string{
								"radium-lovers",
								"polonium-lovers",
								"physics-lovers",
							},
						},
					},
				},
				{
					name: "not-existing opaqueId",
					userID: &userpb.UserId{
						Idp:      existingIdp,
						OpaqueId: "doesnote-xist-4376-b307-cf0a8c2d0d9c",
					},
					want: &userpb.GetUserResponse{
						Status: &rpc.Status{
							Code: 15,
						},
					},
				},
				{
					name: "no opaqueId",
					userID: &userpb.UserId{
						Idp:      existingIdp,
						OpaqueId: "",
					},
					want: &userpb.GetUserResponse{
						Status: &rpc.Status{
							Code: 15,
						},
					},
				},
				{
					name: "not-existing idp",
					userID: &userpb.UserId{
						Idp:      "http://does-not-exist:12345",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					},
					want: &userpb.GetUserResponse{
						Status: &rpc.Status{
							Code: 15,
						},
					},
				},
				{
					name: "no idp",
					userID: &userpb.UserId{
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
					},
					want: &userpb.GetUserResponse{
						Status: &rpc.Status{
							Code: 1,
						},
						User: &userpb.User{
							Username:    "marie",
							Mail:        "marie@example.org",
							DisplayName: "Marie Curie",
							Groups: []string{
								"radium-lovers",
								"polonium-lovers",
								"physics-lovers",
							},
						},
					},
				},
			}

			for _, t := range tests {
				userResp, err := serviceClient.GetUser(ctx, &userpb.GetUserRequest{
					UserId: t.userID,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(t.want.Status.Code).To(Equal(userResp.Status.Code))
				if t.want.User == nil {
					Expect(userResp.User).To(BeNil())
				} else {
					// make sure not to run into a nil pointer error
					Expect(userResp.User).ToNot(BeNil())
					Expect(t.want.User.Username).To(Equal(userResp.User.Username))
					Expect(t.want.User.Mail).To(Equal(userResp.User.Mail))
					Expect(t.want.User.DisplayName).To(Equal(userResp.User.DisplayName))
					Expect(t.want.User.Groups).To(Equal(userResp.User.Groups))
				}
			}
		})
	}

	var assertFindUsersResponses = func() {
		It("finds users by email", func() {
			res, err := serviceClient.FindUsers(ctx, &userpb.FindUsersRequest{Filter: "marie@example.org"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Users)).To(Equal(1))
			user := res.Users[0]
			Expect(user.DisplayName).To(Equal("Marie Curie"))
		})

		It("finds users by displayname", func() {
			res, err := serviceClient.FindUsers(ctx, &userpb.FindUsersRequest{Filter: "Marie Curie"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Users)).To(Equal(1))
			user := res.Users[0]
			Expect(user.Mail).To(Equal("marie@example.org"))
		})

		It("finds users by username", func() {
			res, err := serviceClient.FindUsers(ctx, &userpb.FindUsersRequest{Filter: "marie"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Users)).To(Equal(1))
			user := res.Users[0]
			Expect(user.Mail).To(Equal("marie@example.org"))
		})

		It("finds users by id", func() {
			res, err := serviceClient.FindUsers(ctx, &userpb.FindUsersRequest{Filter: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Users)).To(Equal(1))
			user := res.Users[0]
			Expect(user.Mail).To(Equal("marie@example.org"))
		})
	}

	Describe("the json userprovider", func() {
		BeforeEach(func() {
			dependencies = map[string]string{
				"users": "userprovider-json.toml",
			}
			existingIdp = "localhost:20080"
		})

		assertFindUsersResponses()
		assertGetUserResponses()
		assertGetUserByClaimResponses()
	})

	Describe("the demo userprovider", func() {
		BeforeEach(func() {
			dependencies = map[string]string{
				"users": "userprovider-demo.toml",
			}
			existingIdp = "http://localhost:9998"
		})

		assertGetUserResponses()
		assertFindUsersResponses()
		assertGetUserByClaimResponses()
	})
})
