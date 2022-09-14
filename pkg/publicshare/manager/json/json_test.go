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

package json_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/json"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"golang.org/x/crypto/bcrypt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Json", func() {
	var (
		user1 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "admin",
			},
		}

		sharedResource = &providerv1beta1.ResourceInfo{
			Id: &providerv1beta1.ResourceId{
				StorageId: "storageid",
				OpaqueId:  "opaqueid",
			},
			ArbitraryMetadata: &providerv1beta1.ArbitraryMetadata{
				Metadata: map[string]string{
					"name": "publicshare",
				},
			},
		}
		grant = &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: &providerv1beta1.ResourcePermissions{
					InitiateFileUpload: false,
				},
			},
		}

		m       publicshare.Manager
		tmpFile *os.File
		ctx     context.Context
	)

	Context("with a file persistence layer", func() {

		BeforeEach(func() {
			var err error
			tmpFile, err = ioutil.TempFile("", "reva-unit-test-*.json")
			Expect(err).ToNot(HaveOccurred())

			config := map[string]interface{}{
				"file":         tmpFile.Name(),
				"gateway_addr": "https://localhost:9200",
			}
			m, err = json.NewFile(config)
			Expect(err).ToNot(HaveOccurred())

			ctx = ctxpkg.ContextSetUser(context.Background(), user1)
		})

		AfterEach(func() {
			os.Remove(tmpFile.Name())
		})

		Describe("Dump", func() {
			JustBeforeEach(func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{
					Password: "foo",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("dumps all public shares", func() {
				psharesChan := make(chan *publicshare.WithPassword)
				pshares := []*publicshare.WithPassword{}

				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() {
					for ps := range psharesChan {
						if ps != nil {
							pshares = append(pshares, ps)
						}
					}
					wg.Done()
				}()
				err := m.(publicshare.DumpableManager).Dump(ctx, psharesChan)
				Expect(err).ToNot(HaveOccurred())
				close(psharesChan)
				wg.Wait()
				Eventually(psharesChan).Should(BeClosed())

				Expect(len(pshares)).To(Equal(1))
				Expect(bcrypt.CompareHashAndPassword([]byte(pshares[0].Password), []byte("foo"))).To(Succeed())
				Expect(pshares[0].PublicShare.Creator).To(Equal(user1.Id))
				Expect(pshares[0].PublicShare.ResourceId).To(Equal(sharedResource.Id))
			})
		})

		Describe("Load", func() {
			It("loads shares including state and mountpoint information", func() {
				existingShare, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{
					Password: "foo",
				})
				Expect(err).ToNot(HaveOccurred())

				targetManager, err := json.NewMemory(map[string]interface{}{})
				Expect(err).ToNot(HaveOccurred())

				sharesChan := make(chan *publicshare.WithPassword)

				wg := sync.WaitGroup{}
				wg.Add(2)
				go func() {
					err := targetManager.(publicshare.LoadableManager).Load(ctx, sharesChan)
					Expect(err).ToNot(HaveOccurred())
					wg.Done()
				}()
				go func() {
					sharesChan <- &publicshare.WithPassword{
						Password:    "foo",
						PublicShare: *existingShare,
					}
					close(sharesChan)
					wg.Done()
				}()
				wg.Wait()
				Eventually(sharesChan).Should(BeClosed())

				loadedPublicShare, err := targetManager.GetPublicShare(ctx, user1, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedPublicShare).ToNot(BeNil())
			})
		})
	})

	Context("with a cs3 persistence layer", func() {
		var (
			tmpdir string

			storage metadata.Storage
		)

		BeforeEach(func() {
			var err error
			tmpdir, err = ioutil.TempDir("", "json-publicshare-manager-test")
			Expect(err).ToNot(HaveOccurred())

			err = os.MkdirAll(tmpdir, 0755)
			Expect(err).ToNot(HaveOccurred())

			storage, err = metadata.NewDiskStorage(tmpdir)
			Expect(err).ToNot(HaveOccurred())

			persistence := cs3.New(storage)
			Expect(persistence.Init(context.Background())).To(Succeed())

			m, err = json.New("https://localhost:9200", 11, 60, false, persistence)
			Expect(err).ToNot(HaveOccurred())

			ctx = ctxpkg.ContextSetUser(context.Background(), user1)
		})

		AfterEach(func() {
			if tmpdir != "" {
				os.RemoveAll(tmpdir)
			}
		})
		Describe("CreatePublicShare", func() {
			It("creates public shares", func() {
				ps, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
			})
		})

		Describe("PublicShares", func() {
			It("lists public shares", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))
				Expect(ps[0].ResourceId).To(Equal(sharedResource.Id))
			})

			It("picks up shares from the storage", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				// Reset manager
				p := cs3.New(storage)
				Expect(p.Init(context.Background())).To(Succeed())

				m, err = json.New("https://localhost:9200", 11, 60, false, p)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))
				Expect(ps[0].ResourceId).To(Equal(sharedResource.Id))
			})

			It("refreshes its cache before writing new data", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))

				// Purge file on storage and make sure its mtime is newer than the cache
				path := filepath.Join(tmpdir, "publicshares.json")
				Expect(os.WriteFile(path, []byte("{}"), 0x644)).To(Succeed())
				t := time.Now().Add(5 * time.Minute)
				Expect(os.Chtimes(path, t, t)).To(Succeed())

				_, err = m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err = m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1)) // Make sure the first created public share is gone
			})
		})
	})
})
