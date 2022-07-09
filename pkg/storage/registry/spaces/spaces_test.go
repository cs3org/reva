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

package spaces_test

import (
	"context"
	"encoding/json"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/registry/spaces"
	"github.com/cs3org/reva/v2/pkg/storage/registry/spaces/mocks"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Spaces", func() {
	var (
		handler   storage.Registry
		ctxAlice  context.Context
		fooClient *mocks.StorageProviderClient
		barClient *mocks.StorageProviderClient
		bazClient *mocks.StorageProviderClient

		rules map[string]interface{}

		getClientFunc func(addr string) (spaces.StorageProviderClient, error)

		alice = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		}
	)

	BeforeEach(func() {
		fooClient = &mocks.StorageProviderClient{}
		barClient = &mocks.StorageProviderClient{}
		bazClient = &mocks.StorageProviderClient{}

		fooClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
			func(_ context.Context, req *provider.ListStorageSpacesRequest, _ ...grpc.CallOption) *provider.ListStorageSpacesResponse {
				rid := provider.ResourceId{StorageId: "provider-1", SpaceId: "foospace", OpaqueId: "foospace"}
				spaces := []*provider.StorageSpace{
					{
						Id:        &provider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(rid)},
						Root:      &rid,
						Name:      "Foo space",
						SpaceType: "personal",
						Owner:     alice,
					},
				}
				for _, f := range req.Filters {
					if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID && f.GetId().OpaqueId != "provider-1$foospace!foospace" {
						spaces = []*provider.StorageSpace{}
					}
				}
				return &provider.ListStorageSpacesResponse{
					Status:        &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: spaces,
				}
			}, nil)
		barClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
			func(_ context.Context, req *provider.ListStorageSpacesRequest, _ ...grpc.CallOption) *provider.ListStorageSpacesResponse {
				rid := provider.ResourceId{StorageId: "provider-1", SpaceId: "barspace", OpaqueId: "barspace"}
				spaces := []*provider.StorageSpace{
					{
						Id:        &provider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(rid)},
						Root:      &rid,
						Name:      "Bar space",
						SpaceType: "personal",
						Owner:     alice,
					},
				}
				for _, f := range req.Filters {
					if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID && f.GetId().OpaqueId != "barspace!barspace" {
						spaces = []*provider.StorageSpace{}
					}
				}
				return &provider.ListStorageSpacesResponse{
					Status:        &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: spaces,
				}
			}, nil)
		bazClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
			func(_ context.Context, req *provider.ListStorageSpacesRequest, _ ...grpc.CallOption) *provider.ListStorageSpacesResponse {
				rid1 := provider.ResourceId{StorageId: "provider-1", SpaceId: "bazspace1", OpaqueId: "bazspace1"}
				rid2 := provider.ResourceId{StorageId: "provider-1", SpaceId: "bazspace2", OpaqueId: "bazspace2"}
				space1 := &provider.StorageSpace{
					Id:        &provider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(rid1)},
					Root:      &rid1,
					Name:      "Baz space 1",
					SpaceType: "project",
					Owner:     alice,
				}
				space2 := &provider.StorageSpace{
					Id:        &provider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(rid2)},
					Root:      &rid2,
					Name:      "Baz space 2",
					SpaceType: "project",
					Owner:     alice,
				}
				spaces := []*provider.StorageSpace{space1, space2}
				for _, f := range req.Filters {
					if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
						if f.GetId().OpaqueId == "provider-1$bazspace1!bazspace2" {
							spaces = []*provider.StorageSpace{space1}
						} else if f.GetId().OpaqueId == "provider-1$bazspace2!bazspace2" {
							spaces = []*provider.StorageSpace{space2}
						} else {
							spaces = []*provider.StorageSpace{}
						}
					}
				}
				return &provider.ListStorageSpacesResponse{
					Status:        &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: spaces,
				}
			}, nil)

		getClientFunc = func(addr string) (spaces.StorageProviderClient, error) {
			switch addr {
			case "127.0.0.1:13020":
				return fooClient, nil
			case "127.0.0.1:13021":
				return barClient, nil
			case "127.0.0.1:13022":
				return bazClient, nil
			}
			return nil, fmt.Errorf("Nooooo")
		}

		ctxAlice = ctxpkg.ContextSetUser(context.Background(), alice)
	})

	JustBeforeEach(func() {
		var err error
		handler, err = spaces.New(rules, getClientFunc)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("NewDefault", func() {
		It("returns a new instance", func() {
			_, err := spaces.NewDefault(rules)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("New", func() {
		It("uses the path as the pathtemplate if no template is set (e.g. in cases like the publicstorageprovider which returns a single space)", func() {
			rules = map[string]interface{}{
				"providers": map[string]interface{}{
					"127.0.0.1:13020": map[string]interface{}{
						"spaces": map[string]interface{}{
							"personal": map[string]interface{}{
								"mount_point": "/thepath",
							},
						},
					},
				},
			}

			handler, err := spaces.New(rules, getClientFunc)
			Expect(err).ToNot(HaveOccurred())

			providers, err := handler.ListProviders(ctxAlice, map[string]string{"path": "/thepath"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(1))
			p := providers[0]
			Expect(p.Address).To(Equal("127.0.0.1:13020"))

			spaces := []*provider.StorageSpace{}
			err = json.Unmarshal(p.Opaque.Map["spaces"].Value, &spaces)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(spaces)).To(Equal(1))

			Expect(spaces[0].Opaque.Map["path"].Decoder).To(Equal("plain"))
			spacePath := string(spaces[0].Opaque.Map["path"].Value)
			Expect(spacePath).To(Equal("/thepath"))
		})
	})

	Context("with a simple setup", func() {
		BeforeEach(func() {
			rules = map[string]interface{}{
				"home_provider": "/users/{{.Id.OpaqueId}}",
				"providers": map[string]interface{}{
					"127.0.0.1:13020": map[string]interface{}{
						"spaces": map[string]interface{}{
							"personal": map[string]interface{}{
								"mount_point":   "/users/[a-k]",
								"path_template": "/users/{{.Space.Owner.Username}}",
							},
						},
					},
					"127.0.0.1:13021": map[string]interface{}{
						"spaces": map[string]interface{}{
							"personal": map[string]interface{}{
								"mount_point":   "/users/[l-z]",
								"path_template": "/users/{{.Space.Owner.Username}}",
							},
						},
					},
					"127.0.0.1:13022": map[string]interface{}{
						"spaces": map[string]interface{}{
							"project": map[string]interface{}{
								"mount_point":   "/projects",
								"path_template": "/projects/{{.Space.Name}}",
							},
						},
					},
				},
			}
		})

		Describe("GetProvider", func() {
			It("returns an error when no provider was found", func() {
				space := &provider.StorageSpace{
					Owner: &userpb.User{
						Username: "bob",
					},
					SpaceType: "somethingfancy",
				}
				p, err := handler.GetProvider(ctxAlice, space)
				Expect(err).To(HaveOccurred())
				Expect(p).To(BeNil())
			})

			It("filters by space type", func() {
				space := &provider.StorageSpace{
					SpaceType: "personal",
				}
				p, err := handler.GetProvider(ctxAlice, space)
				Expect(err).ToNot(HaveOccurred())
				Expect(p).ToNot(BeNil())
			})

			It("filters by space type and owner", func() {
				space := &provider.StorageSpace{
					Owner: &userpb.User{
						Username: "alice",
					},
					SpaceType: "personal",
				}
				p, err := handler.GetProvider(ctxAlice, space)
				Expect(err).ToNot(HaveOccurred())
				Expect(p).ToNot(BeNil())
				Expect(p.Address).To(Equal("127.0.0.1:13020"))

				space = &provider.StorageSpace{
					Owner: &userpb.User{
						Username: "zacharias",
					},
					SpaceType: "personal",
				}
				p, err = handler.GetProvider(ctxAlice, space)
				Expect(err).ToNot(HaveOccurred())
				Expect(p).ToNot(BeNil())
				Expect(p.Address).To(Equal("127.0.0.1:13021"))
			})
		})

		Describe("ListProviders", func() {
			It("returns all providers when no filter is set", func() {
				filters := map[string]string{}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(3))
			})

			It("filters by path", func() {
				filters := map[string]string{
					"path": "/projects",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				p := providers[0]
				Expect(p.Address).To(Equal("127.0.0.1:13022"))

				spaces := []*provider.StorageSpace{}
				err = json.Unmarshal(p.Opaque.Map["spaces"].Value, &spaces)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(spaces)).To(Equal(2))

				baz1Found, baz2Found := false, false
				for _, space := range spaces {
					spacePath := string(space.Opaque.Map["path"].Value)
					switch space.Id.OpaqueId {
					case "provider-1$bazspace1!bazspace1":
						baz1Found = true
						Expect(spacePath).To(Equal("/projects/Baz space 1"))
					case "provider-1$bazspace2!bazspace2":
						baz2Found = true
						Expect(spacePath).To(Equal("/projects/Baz space 2"))
					default:
						Fail("unexpected space id")
					}
				}
				Expect(baz1Found).To(BeTrue())
				Expect(baz2Found).To(BeTrue())
			})

			It("returns an empty list when a non-existent id is given", func() {
				filters := map[string]string{
					"storage_id": "invalid",
					"space_id":   "invalid",
					"opaque_id":  "barspace",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(0))
			})

			It("filters by id", func() {
				filters := map[string]string{
					"storage_id": "provider-1",
					"space_id":   "foospace",
					"opaque_id":  "foospace",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				p := providers[0]
				Expect(p.Address).To(Equal("127.0.0.1:13020"))

				spaces := []*provider.StorageSpace{}
				err = json.Unmarshal(p.Opaque.Map["spaces"].Value, &spaces)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(spaces)).To(Equal(1))

				Expect(spaces[0].Id.OpaqueId).To(Equal("provider-1$foospace!foospace"))
				Expect(spaces[0].Opaque.Map["path"].Decoder).To(Equal("plain"))
				spacePath := string(spaces[0].Opaque.Map["path"].Value)
				Expect(spacePath).To(Equal("/users/alice"))

				filters = map[string]string{
					"storage_id": "provider-1",
					"space_id":   "bazspace2",
					"opaque_id":  "bazspace2",
				}
				providers, err = handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				p = providers[0]
				Expect(p.Address).To(Equal("127.0.0.1:13022"))

				err = json.Unmarshal(p.Opaque.Map["spaces"].Value, &spaces)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(spaces)).To(Equal(1))
				Expect(spaces[0].Id.OpaqueId).To(Equal("provider-1$bazspace2!bazspace2"))
				Expect(spaces[0].Opaque.Map["path"].Decoder).To(Equal("plain"))
				spacePath = string(spaces[0].Opaque.Map["path"].Value)
				Expect(spacePath).To(Equal("/projects/Baz space 2"))
			})
		})
	})

	Context("with a nested setup", func() {
		BeforeEach(func() {
			rules = map[string]interface{}{
				"home_provider": "/users/{{.Id.OpaqueId}}",
				"providers": map[string]interface{}{
					"127.0.0.1:13020": map[string]interface{}{
						"spaces": map[string]interface{}{
							"personal": map[string]interface{}{
								"mount_point":   "/foo",
								"path_template": "/foo",
							},
						},
					},
					"127.0.0.1:13021": map[string]interface{}{
						"spaces": map[string]interface{}{
							"personal": map[string]interface{}{
								"mount_point":   "/foo/bar",
								"path_template": "/foo/bar",
							},
						},
					},
					"127.0.0.1:13022": map[string]interface{}{
						"spaces": map[string]interface{}{
							"project": map[string]interface{}{
								"mount_point":   "/foo/bar/baz",
								"path_template": "/foo/bar/baz",
							},
						},
					},
				},
			}
		})

		Describe("ListProviders", func() {
			It("includes all spaces below the requested path", func() {
				filters := map[string]string{
					"path": "/foo",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(3))
			})

			It("includes all spaces below the requested path but not the one above", func() {
				filters := map[string]string{
					"path": "/foo/bar",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(2))
				addresses := []string{}
				for _, p := range providers {
					addresses = append(addresses, p.Address)
				}
				Expect(addresses).To(ConsistOf("127.0.0.1:13021", "127.0.0.1:13022"))
			})

			It("includes the space for the requested path", func() {
				filters := map[string]string{
					"path": "/foo/bar/baz",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13022"))

				filters = map[string]string{
					"path": "/foo/bar/baz/qux",
				}
				providers, err = handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13022"))
			})

			It("includes the space for the requested path", func() {
				filters := map[string]string{
					"path": "/foo/bar/bif",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13021"))
			})
		})
	})
})
