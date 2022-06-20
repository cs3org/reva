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
	"sync"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/json"
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

		m       publicshare.Manager
		tmpFile *os.File
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		tmpFile, err = ioutil.TempFile("", "reva-unit-test-*.json")
		Expect(err).ToNot(HaveOccurred())

		config := map[string]interface{}{
			"file":         tmpFile.Name(),
			"gateway_addr": "https://localhost:9200",
		}
		m, err = json.New(config)
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
			wg.Wait()
			Eventually(psharesChan).Should(BeClosed())

			Expect(len(pshares)).To(Equal(1))
			Expect(bcrypt.CompareHashAndPassword([]byte(pshares[0].Password), []byte("foo"))).To(Succeed())
			Expect(pshares[0].PublicShare.Creator).To(Equal(user1.Id))
			Expect(pshares[0].PublicShare.ResourceId).To(Equal(sharedResource.Id))
		})
	})
})
