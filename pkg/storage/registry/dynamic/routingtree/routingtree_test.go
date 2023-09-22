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
	"github.com/cs3org/reva/pkg/storage/registry/dynamic/routingtree"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Routing Tree", func() {
	var (
		routes = map[string]string{
			"/eos/user/j":               "eoshome-i01",
			"/eos/user/g":               "eoshome-i02",
			"/eos/project/a/atlas":      "eosproject-i00",
			"/eos/project/c/cernbox":    "eosproject-i01",
			"/eos/project/c/cernbox-2":  "eosproject-i01",
			"/cephfs/project/c/cephbox": "cephfs",
		}

		nonLeaf = map[string][]string{
			"/":                 {"eoshome-i01", "eoshome-i02", "eosproject-i00", "eosproject-i01", "cephfs"},
			"/eos":              {"eoshome-i01", "eoshome-i02", "eosproject-i00", "eosproject-i01"},
			"/eos/":             {"eoshome-i01", "eoshome-i02", "eosproject-i00", "eosproject-i01"},
			"/eos/user":         {"eoshome-i01", "eoshome-i02"},
			"/eos/user/":        {"eoshome-i01", "eoshome-i02"},
			"/eos/project":      {"eosproject-i00", "eosproject-i01"},
			"/eos/project/a":    {"eosproject-i00"},
			"/eos/project/c":    {"eosproject-i01"},
			"/cephfs":           {"cephfs"},
			"/cephfs/project/":  {"cephfs"},
			"/cephfs/project/c": {"cephfs"},
		}

		deepRoutes = map[string]string{
			"/eos/project/c/cernbox/a/deep/route":           "eosproject-i01",
			"/eos/user/j/jaferrer/test/folder/":             "eoshome-i01",
			"/cephfs/project/c/cephbox/another/deep/folder": "cephfs",
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
		p   []string
		err error
	)

	BeforeEach(func() {
		t = *routingtree.New(routes)
	})

	Context("resolving providers", func() {
		for r, m := range routes {
			r := r
			m := m
			When("passed an existing leaf route: "+r, func() {
				JustBeforeEach(func() {
					p, err = t.Resolve(r)
				})

				It("should return the correct provider", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(m))
				})
			})
		}

		When("passed a non-existing route", func() {
			JustBeforeEach(func() {
				p, err = t.Resolve("/this/path/does/not/exist")
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		for nl, ps := range nonLeaf {
			nl := nl
			ps := ps
			When("passed an existing non-leaf route: "+nl, func() {
				JustBeforeEach(func() {
					p, err = t.Resolve(nl)
				})

				It("should return the correct providers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(ps))
				})
			})
		}

		for r, wp := range deepRoutes {
			r := r
			wp := wp
			When("passed a deep route: "+r, func() {
				JustBeforeEach(func() {
					p, err = t.Resolve(r)
				})

				It("should return the correct providers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(ConsistOf(wp))
				})
			})
		}

		for _, r := range badRoutes {
			r := r
			When("passed a bad route: "+r, func() {
				JustBeforeEach(func() {
					p, err = t.Resolve(r)
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		}
	})
})
