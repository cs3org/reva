// Copyright 2018-2022 CERN
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
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage/registry/static"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Static", func() {

	totalProviders, rootProviders, eosProviders := 33, 31, 29

	handler, err := static.New(map[string]interface{}{
		"home_provider": "/home",
		"rules": map[string]interface{}{
			"/home": map[string]interface{}{
				"mapping": "/home-{{substr 0 1 .Id.OpaqueId}}",
				"aliases": map[string]string{
					"/home-[a-fg-o]": "home-00-home",
					"/home-[pqrstu]": "home-01-home",
					"/home-[v-z]":    "home-02-home",
				},
			},
			"/MyShares": map[string]interface{}{
				"mapping": "/MyShares-{{substr 0 1 .Id.OpaqueId}}",
				"aliases": map[string]string{
					"/MyShares-[a-fg-o]": "home-00-shares",
					"/MyShares-[pqrstu]": "home-01-shares",
					"/MyShares-[v-z]":    "home-02-shares",
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

	Describe("ListProviders", func() {
		It("lists all providers for user alice", func() {
			providers, err := handler.ListProviders(ctxAlice)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(totalProviders))
		})

		It("lists all providers for user robert", func() {
			providers, err := handler.ListProviders(ctxRobert)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(totalProviders))
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

	Describe("FindProviders for home reference", func() {
		ref := &provider.Reference{Path: "/home/abcd"}

		It("finds all providers for user alice for a home ref", func() {
			providers, err := handler.FindProviders(ctxAlice, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00}))
		})

		It("finds all providers for user robert for a home ref", func() {
			providers, err := handler.FindProviders(ctxRobert, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home01}))
		})
	})

	Describe("FindProviders for eos reference", func() {
		ref := &provider.Reference{Path: "/eos/user/b/bob/xyz"}
		eosUserB := &registrypb.ProviderInfo{
			ProviderPath: "/eos/user/b",
			Address:      "home-00-eos",
		}

		It("finds all providers for user alice for an eos ref", func() {
			providers, err := handler.FindProviders(ctxAlice, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosUserB}))
		})

		It("finds all providers for user robert for an eos ref", func() {
			providers, err := handler.FindProviders(ctxRobert, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosUserB}))
		})
	})

	Describe("FindProviders for project reference", func() {
		ref := &provider.Reference{Path: "/eos/project/pqr"}
		eosProject := &registrypb.ProviderInfo{
			ProviderPath: "/eos/project",
			Address:      "project-00",
		}

		It("finds all providers for user alice for a project ref", func() {
			providers, err := handler.FindProviders(ctxAlice, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosProject}))
		})

		It("finds all providers for user robert for a project ref", func() {
			providers, err := handler.FindProviders(ctxRobert, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{eosProject}))
		})
	})

	Describe("FindProviders for virtual references", func() {
		refEos := &provider.Reference{Path: "/eos"}
		refRoot := &provider.Reference{Path: "/"}

		It("finds all providers for user alice for a virtual eos ref", func() {
			providers, err := handler.FindProviders(ctxAlice, refEos)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(eosProviders))
		})

		It("finds all providers for user robert for a virtual eos ref", func() {
			providers, err := handler.FindProviders(ctxRobert, refEos)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(eosProviders))
		})

		It("finds all providers for user alice for a virtual root ref", func() {
			providers, err := handler.FindProviders(ctxAlice, refRoot)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(rootProviders))
		})

		It("finds all providers for user robert for a virtual root ref", func() {
			providers, err := handler.FindProviders(ctxRobert, refRoot)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(providers)).To(Equal(rootProviders))
		})
	})

	Describe("FindProviders for reference containing ID", func() {
		ref := &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: "123e4567-e89b-12d3-a456-426655440000",
			},
		}
		home00ID := &registrypb.ProviderInfo{
			ProviderId: "123e4567-e89b-12d3-a456-426655440000",
			Address:    "home-00-home",
		}

		It("finds all providers for user alice for ref containing ID", func() {
			providers, err := handler.FindProviders(ctxAlice, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00ID}))
		})

		It("finds all providers for user robert for ref containing ID", func() {
			providers, err := handler.FindProviders(ctxRobert, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(providers).To(Equal([]*registrypb.ProviderInfo{home00ID}))
		})
	})
})
