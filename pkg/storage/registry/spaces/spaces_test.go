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
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/spaces"
	"github.com/cs3org/reva/pkg/storage/registry/spaces/mocks"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Static", func() {
	var (
		handler  storage.Registry
		ctxAlice context.Context
		client   *mocks.StorageProviderClient

		rules map[string]interface{}

		getClientFunc func(addr string) (spaces.StorageProviderClient, error)
	)

	BeforeEach(func() {
		client = &mocks.StorageProviderClient{}
		ctxAlice = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
		})

		getClientFunc = func(addr string) (spaces.StorageProviderClient, error) {
			return client, nil
		}
	})

	JustBeforeEach(func() {
		var err error
		handler, err = spaces.New(rules, getClientFunc)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with a simple setup", func() {
		BeforeEach(func() {
			rules = map[string]interface{}{
				"home_provider": "/users/{{.Id.OpaqueId}}",
				"rules": map[string]interface{}{
					"/users/[a-k]": map[string]interface{}{
						"path_template": "/users/{{.Space.Owner.Username}}",
						"space_type":    "personal",
						"address":       "127.0.0.1:13020",
					},
					"/users/[l-z]": map[string]interface{}{
						"path_template": "/users/{{.Space.Owner.Username}}",
						"space_type":    "personal",
						"address":       "127.0.0.1:13021",
					},
					"/projects": map[string]interface{}{
						"path_template": "/projects/{{.Space.Name}}",
						"space_type":    "project",
						"address":       "127.0.0.1:13022",
					},
				},
			}
		})

		Describe("GetProvider", func() {
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
			Context("path based requests", func() {
				BeforeEach(func() {
					client.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
						&provider.ListStorageSpacesResponse{
							Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
							StorageSpaces: []*provider.StorageSpace{
								{
									Id:   &provider.StorageSpaceId{OpaqueId: "space1!space1"},
									Root: &provider.ResourceId{StorageId: "space1", OpaqueId: "space1"},
									Name: "Space 1",
								},
								{
									Id:   &provider.StorageSpaceId{OpaqueId: "space2!space2"},
									Root: &provider.ResourceId{StorageId: "space2", OpaqueId: "space2"},
									Name: "Space 2",
								},
							},
						}, nil)
				})

				It("filters by path with a simple rule", func() {
					filters := map[string]string{
						"space_path": "/projects",
					}
					providers, err := handler.ListProviders(ctxAlice, filters)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(providers)).To(Equal(1))
					p := providers[0]
					Expect(p.Address).To(Equal("127.0.0.1:13022"))

					spacePaths := map[string]string{}
					err = json.Unmarshal(p.Opaque.Map["space_paths"].Value, &spacePaths)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(spacePaths)).To(Equal(2))
					Expect(spacePaths["space1!space1"]).To(Equal("/projects/Space 1"))
					Expect(spacePaths["space2!space2"]).To(Equal("/projects/Space 2"))
				})
			})

			Context("with id based requests", func() {
				BeforeEach(func() {
					client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *provider.ListStorageSpacesRequest) bool {
						return len(req.Filters) == 2 && // the 2 filters are the space type defined in the rule and the id from the request
							req.Filters[1].Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID &&
							req.Filters[1].GetId().OpaqueId == "space1!space1"
					})).Return(&provider.ListStorageSpacesResponse{
						Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
						StorageSpaces: []*provider.StorageSpace{
							{
								Id:   &provider.StorageSpaceId{OpaqueId: "space1!space1"},
								Root: &provider.ResourceId{StorageId: "space1", OpaqueId: "space1"},
								Name: "Space 1",
							},
						},
					}, nil)
					client.On("ListStorageSpaces", mock.Anything, mock.Anything).Return( // fallback
						&provider.ListStorageSpacesResponse{
							Status:        &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
							StorageSpaces: []*provider.StorageSpace{},
						}, nil)
				})

				It("filters by id", func() {
					filters := map[string]string{
						"storage_id": "space1",
						"opaque_id":  "space1",
					}
					providers, err := handler.ListProviders(ctxAlice, filters)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(providers)).To(Equal(1))
					p := providers[0]
					Expect(p.Address).To(Equal("127.0.0.1:13022"))

					spacePaths := map[string]string{}
					err = json.Unmarshal(p.Opaque.Map["space_paths"].Value, &spacePaths)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(spacePaths)).To(Equal(1))
					Expect(spacePaths["space1!space1"]).To(Equal("/projects/Space 1"))
				})
			})
		})
	})

	Context("with a more complex setup", func() {
		var (
			fooClient *mocks.StorageProviderClient
			barClient *mocks.StorageProviderClient
			bazClient *mocks.StorageProviderClient
		)

		BeforeEach(func() {
			getClientFunc = func(addr string) (spaces.StorageProviderClient, error) {
				switch addr {
				case "127.0.0.1:13022":
					return fooClient, nil
				case "127.0.0.1:13023":
					return barClient, nil
				case "127.0.0.1:13024":
					return bazClient, nil
				}
				return nil, fmt.Errorf("Nooooo")
			}

			rules = map[string]interface{}{
				"home_provider": "/users/{{.Id.OpaqueId}}",
				"rules": map[string]interface{}{
					"/foo": map[string]interface{}{
						"path_template": "/foo",
						"space_type":    "project",
						"address":       "127.0.0.1:13022",
					},
					"/foo/bar": map[string]interface{}{
						"path_template": "/foo/bar",
						"space_type":    "project",
						"address":       "127.0.0.1:13023",
					},
					"/foo/bar/baz": map[string]interface{}{
						"path_template": "/foo/bar/baz",
						"space_type":    "project",
						"address":       "127.0.0.1:13024",
					},
				},
			}

			fooClient = &mocks.StorageProviderClient{}
			barClient = &mocks.StorageProviderClient{}
			bazClient = &mocks.StorageProviderClient{}

			fooClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
				&provider.ListStorageSpacesResponse{
					Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: []*provider.StorageSpace{
						{
							Id:   &provider.StorageSpaceId{OpaqueId: "foospace"},
							Root: &provider.ResourceId{StorageId: "foospace", OpaqueId: "foospace"},
							Name: "Foo space",
						},
					},
				}, nil)
			barClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
				&provider.ListStorageSpacesResponse{
					Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: []*provider.StorageSpace{
						{
							Id:   &provider.StorageSpaceId{OpaqueId: "barspace"},
							Root: &provider.ResourceId{StorageId: "barspace", OpaqueId: "barspace"},
							Name: "Bar space",
						},
					},
				}, nil)
			bazClient.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(
				&provider.ListStorageSpacesResponse{
					Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
					StorageSpaces: []*provider.StorageSpace{
						{
							Id:   &provider.StorageSpaceId{OpaqueId: "bazspace"},
							Root: &provider.ResourceId{StorageId: "bazspace", OpaqueId: "bazspace"},
							Name: "Baz space",
						},
					},
				}, nil)
		})

		Describe("ListProviders", func() {
			It("includes all spaces below the requested path", func() {
				filters := map[string]string{
					"space_path": "/foo",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(3))
			})

			It("includes all spaces below the requested path but not the one above", func() {
				filters := map[string]string{
					"space_path": "/foo/bar",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(2))
				addresses := []string{}
				for _, p := range providers {
					addresses = append(addresses, p.Address)
				}
				Expect(addresses).To(ConsistOf("127.0.0.1:13023", "127.0.0.1:13024"))
			})

			It("includes the space for the requested path", func() {
				filters := map[string]string{
					"space_path": "/foo/bar/baz",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13024"))

				filters = map[string]string{
					"space_path": "/foo/bar/baz/qux",
				}
				providers, err = handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13024"))
			})

			It("includes the space for the requested path", func() {
				filters := map[string]string{
					"space_path": "/foo/bar/bif",
				}
				providers, err := handler.ListProviders(ctxAlice, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(providers)).To(Equal(1))
				Expect(providers[0].Address).To(Equal("127.0.0.1:13023"))
			})
		})
	})
})
