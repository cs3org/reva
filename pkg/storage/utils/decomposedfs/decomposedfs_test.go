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
	"github.com/stretchr/testify/mock"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decomposed", func() {
	var (
		env *helpers.TestEnv

		ref *provider.Reference
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: "/dir1",
			},
		}
	})

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

	Describe("NewDefault", func() {
		It("works", func() {
			bs := &treemocks.Blobstore{}
			_, err := decomposedfs.NewDefault(map[string]interface{}{
				"root": env.Root,
			}, bs)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Delete", func() {
		Context("with insufficient permissions", func() {
			It("returns an error", func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

				err := env.Fs.Delete(env.Ctx, ref)

				Expect(err).To(MatchError(ContainSubstring("permission denied")))
			})
		})

		Context("with sufficient permissions", func() {
			JustBeforeEach(func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
			})

			It("does not (yet) delete the blob from the blobstore", func() {
				err := env.Fs.Delete(env.Ctx, ref)

				Expect(err).ToNot(HaveOccurred())
				env.Blobstore.AssertNotCalled(GinkgoT(), "Delete", mock.AnythingOfType("string"))
			})
		})
	})
})
