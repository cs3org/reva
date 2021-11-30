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
		handler   storage.Registry
		ctxAlice  context.Context
		fooClient *mocks.StorageProviderClient
		barClient *mocks.StorageProviderClient
		bazClient *mocks.StorageProviderClient

		rules map[string]interface{}

		getClientFunc func(addr string) (spaces.StorageProviderClient, error)
	)

	BeforeEach(func() {
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
						Id:   &provider.StorageSpaceId{OpaqueId: "bazspace1"},
						Root: &provider.ResourceId{StorageId: "bazspace1", OpaqueId: "bazspace1"},
						Name: "Baz space 1",
					},
					{
						Id:   &provider.StorageSpaceId{OpaqueId: "bazspace2"},
						Root: &provider.ResourceId{StorageId: "bazspace2", OpaqueId: "bazspace2"},
						Name: "Baz space 2",
					},
				},
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

		ctxAlice = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
		})
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
				"rules": map[string]interface{}{
					"/thepath": map[string]interface{}{
						"space_type": "personal",
						"address":    "127.0.0.1:13020"},
				},
			}

			handler, err := spaces.New(rules, getClientFunc)
			Expect(err).ToNot(HaveOccurred())

			providers, err := handler.ListProviders(ctxAlice, map[string]string{"path": "/thepath"})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(1))
			p := providers[0]
			Expect(p.Address).To(Equal("127.0.0.1:13020"))

			spacePaths := map[string]string{}
			err = json.Unmarshal(p.Opaque.Map["space_paths"].Value, &spacePaths)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(spacePaths)).To(Equal(1))
			Expect(spacePaths["foospace"]).To(Equal("/thepath"))
		})
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
			It("filters by path with a simple rule", func() {
				filters := map[string]string{
					"path": "/projects",
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
				Expect(spacePaths["bazspace1"]).To(Equal("/projects/Baz space 1"))
				Expect(spacePaths["bazspace2"]).To(Equal("/projects/Baz space 2"))
			})
		})
	})

	Context("with a more complex setup", func() {
		BeforeEach(func() {
			rules = map[string]interface{}{
				"home_provider": "/users/{{.Id.OpaqueId}}",
				"rules": map[string]interface{}{
					"/foo": map[string]interface{}{
						"path_template": "/foo",
						"space_type":    "project",
						"address":       "127.0.0.1:13020",
					},
					"/foo/bar": map[string]interface{}{
						"path_template": "/foo/bar",
						"space_type":    "project",
						"address":       "127.0.0.1:13021",
					},
					"/foo/bar/baz": map[string]interface{}{
						"path_template": "/foo/bar/baz",
						"space_type":    "project",
						"address":       "127.0.0.1:13022",
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
