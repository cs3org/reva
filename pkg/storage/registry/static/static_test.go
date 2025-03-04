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

package static_test

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/storage/registry/static"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Static", func() {

	rootProviders, eosProviders := 31, 29

	handler, err := static.New(map[string]interface{}{
		"home_provider": "/home",
		"rules": map[string]interface{}{
			"/home": map[string]interface{}{
				"mapping": "/home-{{substr 0 1 .Id.OpaqueId}}",
				"aliases": map[string]interface{}{
					"/home-[a-fg-o]": map[string]string{
						"address": "home-00-home",
					},
					"/home-[pqrstu]": map[string]string{
						"address": "home-01-home",
					},
					"/home-[v-z]": map[string]string{
						"address": "home-02-home",
					},
				},
			},
			"/MyShares": map[string]interface{}{
				"mapping": "/MyShares-{{substr 0 1 .Id.OpaqueId}}",
				"aliases": map[string]interface{}{
					"/MyShares-[a-fg-o]": map[string]string{
						"address": "home-00-shares",
					},
					"/MyShares-[pqrstu]": map[string]string{
						"address": "home-01-shares",
					},
					"/MyShares-[v-z]": map[string]string{
						"address": "home-02-shares",
					},
				},
			},
			"/eos/user/[a-fg-o]": map[string]interface{}{
				"address": "home-00-eos",
			},
			"/eos/user/[pqrstu]": map[string]interface{}{
				"address": "home-01-eos",
			},
			"/eos/user/[v-z]": map[string]interface{}{
				"address": "home-02-eos",
			},
			"/eos/project": map[string]interface{}{
				"address": "project-00",
			},
			"/eos/media": map[string]interface{}{
				"address": "media-00",
			},
			"123e4567-e89b-12d3-a456-426655440000": map[string]interface{}{
				"address": "home-00-home",
			},
			"123e4567-e89b-12d3-a456-426655440001": map[string]interface{}{
				"address": "home-01-home",
			},
			"/eos/": map[string]interface{}{
				"address": "unspecific-rule-that-should-never-been-hit",
			},
		},
	})
	Expect(err).ToNot(HaveOccurred())

	ctxAlice := ctxpkg.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: "alice",
		},
	})
	ctxRobert := ctxpkg.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: "robert",
		},
	})

	Describe("GetProvider", func() {
		It("get provider for user alice", func() {
			provider, err := handler.GetProvider(ctxAlice, &provider.StorageSpace{
				Owner: &userpb.User{
					Id: &userpb.UserId{
						OpaqueId: "alice",
					},
				},
				SpaceType: "personal",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(provider).To(Not(BeNil()))
		})

		It("get provider for user robert", func() {
			provider, err := handler.GetProvider(ctxRobert, &provider.StorageSpace{
				Owner: &userpb.User{
					Id: &userpb.UserId{
						OpaqueId: "robert",
					},
				},
				SpaceType: "personal",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(provider).To(Not(BeNil()))
		})
	})

	home00 := &registrypb.ProviderInfo{
		ProviderPath: "/home",
		Address:      "home-00-home",
	}
	home01 := &registrypb.ProviderInfo{
		ProviderPath: "/home",
		Address:      "home-01-home",
	}
	/*
		Describe("GetHome", func() {
			It("get the home provider for user alice", func() {
				home, err := handler.GetHome(ctxAlice)
				Expect(err).ToNot(HaveOccurred())
				Expect(home).To(Equal(home00))
			})

			It("get the home provider for user robert", func() {
				home, err := handler.GetHome(ctxRobert)
				Expect(err).ToNot(HaveOccurred())
				Expect(home).To(Equal(home01))
			})
		})
	*/

	Describe("FindProviders for home path filter", func() {
		filters := map[string]string{
			"path": "/home/abcd",
		}

		It("finds all providers for user alice for a home path filter", func() {
			providers, err := handler.ListProviders(ctxAlice, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00}))
		})

		It("finds all providers for user robert for a home path filter", func() {
			providers, err := handler.ListProviders(ctxRobert, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home01}))
		})
	})

	Describe("FindProviders for eos path filter", func() {
		filters := map[string]string{
			"path": "/eos/user/b/bob/xyz",
		}
		eosUserB := &registrypb.ProviderInfo{
			ProviderPath: "/eos/user/b",
			Address:      "home-00-eos",
		}

		It("finds all providers for user alice for an eos path filter", func() {
			providers, err := handler.ListProviders(ctxAlice, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosUserB}))
		})

		It("finds all providers for user robert for an eos path filter", func() {
			providers, err := handler.ListProviders(ctxRobert, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosUserB}))
		})
	})

	Describe("FindProviders for project reference", func() {
		filters := map[string]string{
			"path": "/eos/project/pqr",
		}
		eosProject := &registrypb.ProviderInfo{
			ProviderPath: "/eos/project",
			Address:      "project-00",
		}

		It("finds all providers for user alice for a project path filter", func() {
			providers, err := handler.ListProviders(ctxAlice, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosProject}))
		})

		It("finds all providers for user robert for a project path filter", func() {
			providers, err := handler.ListProviders(ctxRobert, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosProject}))
		})
	})

	Describe("FindProviders for virtual references", func() {
		filtersEos := map[string]string{
			"path": "/eos",
		}
		filtersRoot := map[string]string{
			"path": "/",
		}

		It("finds all providers for user alice for a virtual eos path filter", func() {
			providers, err := handler.ListProviders(ctxAlice, filtersEos)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(eosProviders))
		})

		It("finds all providers for user robert for a virtual eos path filter", func() {
			providers, err := handler.ListProviders(ctxRobert, filtersEos)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(eosProviders))
		})

		It("finds all providers for user alice for a virtual root path filter", func() {
			providers, err := handler.ListProviders(ctxAlice, filtersRoot)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(rootProviders))
		})

		It("finds all providers for user robert for a virtual root path filter", func() {
			providers, err := handler.ListProviders(ctxRobert, filtersRoot)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(rootProviders))
		})
	})

	Describe("FindProviders for reference containing ID", func() {
		filters := map[string]string{
			"storage_id": "123e4567-e89b-12d3-a456-426655440000",
		}
		home00ID := &registrypb.ProviderInfo{
			ProviderId: "123e4567-e89b-12d3-a456-426655440000",
			Address:    "home-00-home",
		}

		It("finds all providers for user alice for filters containing ID", func() {
			providers, err := handler.ListProviders(ctxAlice, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00ID}))
		})

		It("finds all providers for user robert for filters containing ID", func() {
			providers, err := handler.ListProviders(ctxRobert, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00ID}))
		})
	})
})
