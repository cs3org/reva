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

package routingtree_test

import (
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"

	"github.com/cs3org/reva/pkg/storage/registry/dynamic/routingtree"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Routing Tree", func() {
	var (
		routes = map[string]string{
			"/eos/user/j":               "eoshome-i01",
			"/eos/user/g":               "eoshome-i02",
			"/eos/project/a":            "eosproject-i00",
			"/eos/project/c":            "eosproject-i01",
			"/cephfs/project/c/cephbox": "cephfs",
		}

		providers = map[string]*registrypb.ProviderInfo{
			"eoshome-i01": {
				ProviderId:   "eoshome-i01",
				ProviderPath: "/eos/user/j",
			},
			"eoshome-i02": {
				ProviderId:   "eoshome-i02",
				ProviderPath: "/eos/user/g",
			},
			"eosproject-i00": {
				ProviderId:   "eosproject-i00",
				ProviderPath: "/eos/project/a",
			},
			"eosproject-i01": {
				ProviderId:   "eosproject-i01",
				ProviderPath: "/eos/project/c",
			},
			"cephfs": {
				ProviderId:   "cephfs",
				ProviderPath: "/cephfs/project/c/cephbox",
			},
		}

		leaf = map[string]*registrypb.ProviderInfo{
			"/eos/user/j":               providers["eoshome-i01"],
			"/eos/user/g":               providers["eoshome-i02"],
			"/eos/project/a":            providers["eosproject-i00"],
			"/eos/project/c":            providers["eosproject-i01"],
			"/cephfs/project/c/cephbox": providers["cephfs"],
		}

		nonLeaf = map[string][]*registrypb.ProviderInfo{
			"/":                 {providers["eoshome-i01"], providers["eoshome-i02"], providers["eosproject-i00"], providers["eosproject-i01"], providers["cephfs"]},
			"/eos":              {providers["eoshome-i01"], providers["eoshome-i02"], providers["eosproject-i00"], providers["eosproject-i01"]},
			"/eos/":             {providers["eoshome-i01"], providers["eoshome-i02"], providers["eosproject-i00"], providers["eosproject-i01"]},
			"/eos/user":         {providers["eoshome-i01"], providers["eoshome-i02"]},
			"/eos/user/":        {providers["eoshome-i01"], providers["eoshome-i02"]},
			"/eos/project":      {providers["eosproject-i00"], providers["eosproject-i01"]},
			"/cephfs":           {providers["cephfs"]},
			"/cephfs/project/":  {providers["cephfs"]},
			"/cephfs/project/c": {providers["cephfs"]},
		}

		deepRoutes = map[string]*registrypb.ProviderInfo{
			"/eos/project/c/cernbox/a/deep/route":           providers["eosproject-i01"],
			"/eos/user/j/jaferrer/test/folder/":             providers["eoshome-i01"],
			"/cephfs/project/c/cephbox/another/deep/folder": providers["cephfs"],
		}

		badRoutes = []string{
			"",
			"badroute",
			"/badroute",
			"/eos/badroute",
			"/eos/project/badroute",
			"/eos/user/xyz",
			"/eos/user/very/long/bad/route/",
			"/cephfs/bad",
			"/cephfs/project/asdf",
		}

		t   routingtree.RoutingTree
		p   []*registrypb.ProviderInfo
		err error
	)

	BeforeEach(func() {
		t = *routingtree.New(routes)
	})

	Context("resolving providers", func() {
		for r, ps := range leaf {
			r := r
			ps := ps
			When("passed an existing leaf route: "+r, func() {
				It("should return the correct provider", func() {
					p, err = t.Resolve(r)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(ps))
				})
			})
		}

		When("passed a non-existing route", func() {
			It("should return an error", func() {
				p, err = t.Resolve("/this/path/does/not/exist")
				Expect(err).To(HaveOccurred())
			})
		})

		for nl, ps := range nonLeaf {
			nl := nl
			ps := ps
			When("passed an existing non-leaf route: "+nl, func() {
				It("should return the correct providers", func() {
					p, err = t.Resolve(nl)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(ps))
				})
			})
		}

		for r, wp := range deepRoutes {
			r := r
			wp := wp
			When("passed a deep route: "+r, func() {
				It("should return the correct providers", func() {
					p, err = t.Resolve(r)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(wp))
				})
			})
		}

		for _, r := range badRoutes {
			r := r
			When("passed a bad route: "+r, func() {
				It("should return an error", func() {
					p, err = t.Resolve(r)
					Expect(err).To(HaveOccurred())
				})
			})
		}
	})
})
