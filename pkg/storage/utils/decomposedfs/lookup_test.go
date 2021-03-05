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

package decomposedfs_test

import (
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lookup", func() {
	var (
		env *helpers.TestEnv
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("Path", func() {
		It("returns the path including a leading slash", func() {
			n, err := env.Lookup.NodeFromPath(env.Ctx, "/dir1/file1")
			Expect(err).ToNot(HaveOccurred())

			path, err := env.Lookup.Path(env.Ctx, n)
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/dir1/file1"))
		})
	})
})
