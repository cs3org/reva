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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs"
	treemocks "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree/mocks"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/tests/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decomposed", func() {
	var (
		options map[string]interface{}
		ctx     context.Context
		tmpRoot string
		fs      storage.FS
	)

	BeforeEach(func() {
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"root":         tmpRoot,
			"share_folder": "/Shares",
			"enable_home":  false,
			"user_layout":  "{{.Id.OpaqueId}}",
			"owner":        "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
		}
		u := &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			Username:    "test",
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
			Groups: []string{
				"radium-lovers",
				"polonium-lovers",
				"physics-lovers",
			},
		}
		ctx = user.ContextSetUser(context.Background(), u)

		bs := &treemocks.Blobstore{}
		fs, err = decomposedfs.NewDefault(options, bs)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("concurrent", func() {
		Describe("Upload", func() {
			var (
				f, f1 *os.File
			)

			BeforeEach(func() {
				// Prepare two test files for upload
				err := ioutil.WriteFile(fmt.Sprintf("%s/%s", tmpRoot, "f.lol"), []byte("test"), 0644)
				Expect(err).ToNot(HaveOccurred())
				f, err = os.Open(fmt.Sprintf("%s/%s", tmpRoot, "f.lol"))
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(fmt.Sprintf("%s/%s", tmpRoot, "f1.lol"), []byte("another run"), 0644)
				Expect(err).ToNot(HaveOccurred())
				f1, err = os.Open(fmt.Sprintf("%s/%s", tmpRoot, "f1.lol"))
				Expect(err).ToNot(HaveOccurred())
			})

			PIt("generates two revisions", func() {
				// runtime.GOMAXPROCS(1) // uncomment to remove concurrency and see revisions working.
				wg := &sync.WaitGroup{}
				wg.Add(2)

				// upload file with contents: "test"
				go func(wg *sync.WaitGroup) {
					_ = fs.Upload(ctx, &provider.Reference{
						Spec: &provider.Reference_Path{Path: "uploaded.txt"},
					}, f)
					wg.Done()
				}(wg)

				// upload file with contents: "another run"
				go func(wg *sync.WaitGroup) {
					_ = fs.Upload(ctx, &provider.Reference{
						Spec: &provider.Reference_Path{Path: "uploaded.txt"},
					}, f1)
					wg.Done()
				}(wg)

				// this test, by the way the oCIS storage is implemented, is non-deterministic, and the contents
				// of uploaded.txt will change on each run depending on which of the 2 routines above makes it
				// first into the scheduler. In order to make it deterministic, we have to consider the Upload impl-
				// ementation and we can leverage concurrency and add locks only when the destination path are the
				// same for 2 uploads.

				wg.Wait()
				revisions, err := fs.ListRevisions(ctx, &provider.Reference{
					Spec: &provider.Reference_Path{Path: "uploaded.txt"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(revisions)).To(Equal(1))

				_, err = ioutil.ReadFile(path.Join(tmpRoot, "nodes", "root", "uploaded.txt"))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("CreateDir", func() {
			It("handle already existing directories", func() {
				for i := 0; i < 10; i++ {
					go func() {
						err := fs.CreateDir(ctx, "fightforit")
						if err != nil {
							rinfo, err := fs.GetMD(ctx, &provider.Reference{
								Spec: &provider.Reference_Path{Path: "fightforit"},
							}, nil)
							Expect(err).ToNot(HaveOccurred())
							Expect(rinfo).ToNot(BeNil())
						}
					}()
				}
			})
		})
	})
})
