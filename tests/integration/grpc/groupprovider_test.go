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
	"os"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	jwt "github.com/cs3org/reva/v2/pkg/token/manager/jwt"
	"google.golang.org/grpc/metadata"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("group providers", func() {
	var (
		dependencies []RevadConfig
		revads       map[string]*Revad

		existingIdp string

		ctx           context.Context
		serviceClient grouppb.GroupAPIClient
	)

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		// Add auth token
		user := &userpb.User{
			Id: &userpb.UserId{
				Idp:      existingIdp,
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
		}
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		scope, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user, scope)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, user)

		revads, err = startRevads(dependencies, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		serviceClient, err = pool.GetGroupProviderServiceClient(revads["groups"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentSpecReport().Failed())).To(Succeed())
		}
	})

	var assertGetGroupByClaimResponses = func() {
		It("gets groups by claim as expected", func() {
			tests := map[string]string{
				"group_name":   "violin-haters",
				"display_name": "Violin Haters",
			}

			for claim, value := range tests {
				group, err := serviceClient.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{Claim: claim, Value: value})
				Expect(err).ToNot(HaveOccurred())
				Expect(group.Group).ToNot(BeNil())
				Expect(group.Group.DisplayName).To(Equal("Violin Haters"))
				Expect(group.Group.Id.OpaqueId).To(Equal("dd58e5ec-842e-498b-8800-61f2ec6f911f"))
			}
		})
	}

	var assertGetGroupResponses = func() {
		It("gets groups as expected", func() {
			tests := []struct {
				name    string
				groupID *grouppb.GroupId
				want    *grouppb.GetGroupResponse
			}{
				{
					name: "simple",
					groupID: &grouppb.GroupId{
						Idp:      existingIdp,
						OpaqueId: "6040aa17-9c64-4fef-9bd0-77234d71bad0",
					},
					want: &grouppb.GetGroupResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_OK,
						},
						Group: &grouppb.Group{
							GroupName:   "sailing-lovers",
							Mail:        "marie@example.org",
							DisplayName: "Sailing Lovers",
							Members: []*userpb.UserId{
								{
									OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
									Idp:      "http://localhost:20080",
								},
							},
						},
					},
				},
				{
					name: "not-existing opaqueId",
					groupID: &grouppb.GroupId{
						Idp:      existingIdp,
						OpaqueId: "doesnote-xist-4376-b307-cf0a8c2d0d9c",
					},
					want: &grouppb.GetGroupResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_NOT_FOUND,
						},
					},
				},
				{
					name: "no opaqueId",
					groupID: &grouppb.GroupId{
						Idp: existingIdp,
					},
					want: &grouppb.GetGroupResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_NOT_FOUND,
						},
					},
				},
				{
					name: "not-existing idp",
					groupID: &grouppb.GroupId{
						Idp:      "http://does-not-exist:12345",
						OpaqueId: "262982c1-2362-4afa-bfdf-8cbfef64a06e",
					},
					want: &grouppb.GetGroupResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_NOT_FOUND,
						},
					},
				},
				{
					name: "no idp",
					groupID: &grouppb.GroupId{
						OpaqueId: "262982c1-2362-4afa-bfdf-8cbfef64a06e",
					},
					want: &grouppb.GetGroupResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_OK,
						},
						Group: &grouppb.Group{
							GroupName:   "physics-lovers",
							DisplayName: "Physics Lovers",
							Members: []*userpb.UserId{
								{
									OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
									Idp:      "http://localhost:20080",
								}, {
									OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
									Idp:      "http://localhost:20080",
								}, {
									OpaqueId: "932b4540-8d16-481e-8ef4-588e4b6b151c",
									Idp:      "http://localhost:20080",
								},
							},
						},
					},
				},
			}

			for _, t := range tests {
				groupResp, err := serviceClient.GetGroup(ctx, &grouppb.GetGroupRequest{
					GroupId: t.groupID,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(t.want.Status.Code).To(Equal(groupResp.Status.Code))
				if t.want.Group == nil {
					Expect(groupResp.Group).To(BeNil())
				} else {
					// make sure not to run into a nil pointer error
					Expect(groupResp.Group).ToNot(BeNil())
					Expect(t.want.Group.GroupName).To(Equal(groupResp.Group.GroupName))
					Expect(t.want.Group.DisplayName).To(Equal(groupResp.Group.DisplayName))
					if len(t.want.Group.Members) == 1 {
						Expect(t.want.Group.Members[0].Idp).To(Equal(groupResp.Group.Members[0].Idp))
						Expect(t.want.Group.Members[0].OpaqueId).To(Equal(groupResp.Group.Members[0].OpaqueId))
					} else {
						Expect(len(t.want.Group.Members)).To(Equal(len(groupResp.Group.Members)))
					}
				}
			}
		})
	}

	var assertFindGroupsResponses = func() {
		It("finds groups by displayname", func() {
			res, err := serviceClient.FindGroups(ctx, &grouppb.FindGroupsRequest{Filter: "Physics Lovers"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Groups)).To(Equal(1))
			group := res.Groups[0]
			Expect(group.Id.OpaqueId).To(Equal("262982c1-2362-4afa-bfdf-8cbfef64a06e"))
		})

		It("finds groups by name", func() {
			res, err := serviceClient.FindGroups(ctx, &grouppb.FindGroupsRequest{Filter: "physics-lovers"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Groups)).To(Equal(1))
			group := res.Groups[0]
			Expect(group.Id.OpaqueId).To(Equal("262982c1-2362-4afa-bfdf-8cbfef64a06e"))
		})

		It("finds groups by id", func() {
			res, err := serviceClient.FindGroups(ctx, &grouppb.FindGroupsRequest{Filter: "262982c1-2362-4afa-bfdf-8cbfef64a06e"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Groups)).To(Equal(1))
			group := res.Groups[0]
			Expect(group.Id.OpaqueId).To(Equal("262982c1-2362-4afa-bfdf-8cbfef64a06e"))
		})
	}

	Describe("the json groupprovider", func() {
		BeforeEach(func() {
			dependencies = []RevadConfig{
				{
					Name:   "groups",
					Config: "groupprovider-json.toml",
				},
			}
			existingIdp = "http://localhost:20080"
		})

		assertFindGroupsResponses()
		assertGetGroupResponses()
		assertGetGroupByClaimResponses()
	})

	Describe("the ldap groupprovider", func() {
		runldap := os.Getenv("RUN_LDAP_TESTS")
		BeforeEach(func() {
			if runldap == "" {
				Skip("Skipping LDAP tests")
			}
			dependencies = []RevadConfig{
				{
					Name:   "groups",
					Config: "groupprovider-ldap.toml",
				},
			}
			existingIdp = "http://localhost:20080"
		})

		assertFindGroupsResponses()
		assertGetGroupResponses()
		assertGetGroupByClaimResponses()
	})
})
