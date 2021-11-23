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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	testhelpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Decomposed", func() {
	var (
		env *testhelpers.TestEnv
	)

	BeforeEach(func() {
		var err error
		env, err = testhelpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			os.RemoveAll(env.Root)
		}
	})

	Describe("concurrent", func() {
		Describe("Upload", func() {
			var (
				f, f1 *os.File
			)

			BeforeEach(func() {
				// Prepare two test files for upload
				err := ioutil.WriteFile(fmt.Sprintf("%s/%s", env.Root, "f.lol"), []byte("test"), 0644)
				Expect(err).ToNot(HaveOccurred())
				f, err = os.Open(fmt.Sprintf("%s/%s", env.Root, "f.lol"))
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(fmt.Sprintf("%s/%s", env.Root, "f1.lol"), []byte("another run"), 0644)
				Expect(err).ToNot(HaveOccurred())
				f1, err = os.Open(fmt.Sprintf("%s/%s", env.Root, "f1.lol"))
				Expect(err).ToNot(HaveOccurred())
			})

			PIt("generates two revisions", func() {
				// runtime.GOMAXPROCS(1) // uncomment to remove concurrency and see revisions working.
				wg := &sync.WaitGroup{}
				wg.Add(2)

				// upload file with contents: "test"
				go func(wg *sync.WaitGroup) {
					_ = env.Fs.Upload(env.Ctx, &provider.Reference{Path: "uploaded.txt"}, f)
					wg.Done()
				}(wg)

				// upload file with contents: "another run"
				go func(wg *sync.WaitGroup) {
					_ = env.Fs.Upload(env.Ctx, &provider.Reference{Path: "uploaded.txt"}, f1)
					wg.Done()
				}(wg)

				// this test, by the way the oCIS storage is implemented, is non-deterministic, and the contents
				// of uploaded.txt will change on each run depending on which of the 2 routines above makes it
				// first into the scheduler. In order to make it deterministic, we have to consider the Upload impl-
				// ementation and we can leverage concurrency and add locks only when the destination path are the
				// same for 2 uploads.

				wg.Wait()
				revisions, err := env.Fs.ListRevisions(env.Ctx, &provider.Reference{Path: "uploaded.txt"})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(revisions)).To(Equal(1))

				_, err = ioutil.ReadFile(path.Join(env.Root, "nodes", "root", "uploaded.txt"))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("CreateDir", func() {
			JustBeforeEach(func() {
				env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
				env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(provider.ResourcePermissions{
					Stat: true,
				}, nil)
			})
			It("handle already existing directories", func() {
				var numIterations = 10
				wg := &sync.WaitGroup{}
				wg.Add(numIterations)
				for i := 0; i < numIterations; i++ {
					go func(wg *sync.WaitGroup) {
						defer GinkgoRecover()
						defer wg.Done()
						ref := &provider.Reference{
							ResourceId: env.SpaceRootRes,
							Path:       "./fightforit",
						}
						if err := env.Fs.CreateDir(env.Ctx, ref); err != nil {
							Expect(err).To(MatchError(ContainSubstring("already exists")))
							rinfo, err := env.Fs.GetMD(env.Ctx, ref, nil)
							Expect(err).ToNot(HaveOccurred())
							Expect(rinfo).ToNot(BeNil())
						}
					}(wg)
				}
				wg.Wait()
			})
		})
	})
})
