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

package lookup_test

import (
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lookup", func() {
	var (
		env *helpers.TestEnv
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("Node from path", func() {
		It("returns the path including a leading slash", func() {
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			path, err := env.Lookup.Path(env.Ctx, n, func(n *node.Node) bool { return true })
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/file1"))
		})
	})

	Describe("Node From Resource only by path", func() {
		It("returns the path including a leading slash and the space root is set", func() {
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/subdir1/file2",
			})
			Expect(err).ToNot(HaveOccurred())

			path, err := env.Lookup.Path(env.Ctx, n, func(n *node.Node) bool { return true })
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/subdir1/file2"))
			Expect(n.SpaceRoot.Name).To(Equal("userid"))
			Expect(n.SpaceRoot.ParentID).To(Equal("root"))
		})
	})

	Describe("Node From Resource only by id", func() {
		It("returns the path including a leading slash and the space root is set", func() {
			// do a node lookup by path
			nRef, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/file1",
			})
			Expect(err).ToNot(HaveOccurred())

			// try to find the same node by id
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: nRef.ID}})
			Expect(err).ToNot(HaveOccurred())
			Expect(n.SpaceRoot).ToNot(BeNil())

			// Check if we got the right node and spaceRoot
			path, err := env.Lookup.Path(env.Ctx, n, func(n *node.Node) bool { return true })
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/file1"))
			Expect(n.SpaceRoot.Name).To(Equal("userid"))
			Expect(n.SpaceRoot.ParentID).To(Equal("root"))
		})
	})

	Describe("Node From Resource by id and relative path", func() {
		It("returns the path including a leading slash and the space root is set", func() {
			// do a node lookup by path for the parent
			nRef, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1",
			})
			Expect(err).ToNot(HaveOccurred())

			// try to find the child node by parent id and relative path
			n, err := env.Lookup.NodeFromResource(env.Ctx, &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: nRef.ID}, Path: "./file1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(n.SpaceRoot).ToNot(BeNil())

			// Check if we got the right node and spaceRoot
			path, err := env.Lookup.Path(env.Ctx, n, func(n *node.Node) bool { return true })
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/file1"))
			Expect(n.SpaceRoot.Name).To(Equal("userid"))
			Expect(n.SpaceRoot.ParentID).To(Equal("root"))
		})
	})
})
