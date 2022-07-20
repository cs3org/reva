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

package jsoncs3_test

import (
	"context"
	"io/ioutil"
	"os"
	"sync"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/share/manager/json"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

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
		user2 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "einstein",
			},
		}

		sharedResource = &providerv1beta1.ResourceInfo{
			Id: &providerv1beta1.ResourceId{
				StorageId: "storageid",
				OpaqueId:  "opaqueid",
			},
		}

		m          share.Manager
		tmpFile    *os.File
		ctx        context.Context
		granteeCtx context.Context
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
		granteeCtx = ctxpkg.ContextSetUser(context.Background(), user2)
	})

	AfterEach(func() {
		os.Remove(tmpFile.Name())
	})

	Describe("Dump", func() {
		JustBeforeEach(func() {
			share, err := m.Share(ctx, sharedResource, &collaboration.ShareGrant{
				Grantee: &providerv1beta1.Grantee{
					Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER,
					Id:   &providerv1beta1.Grantee_UserId{UserId: user2.Id},
				},
			})
			Expect(err).ToNot(HaveOccurred())

			rs, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Id}})
			Expect(err).ToNot(HaveOccurred())
			Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
			rs.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
			rs.MountPoint = &providerv1beta1.Reference{Path: "newPath/"}

			_, err = m.UpdateReceivedShare(granteeCtx,
				rs, &fieldmaskpb.FieldMask{Paths: []string{"state", "mount_point"}})
			Expect(err).ToNot(HaveOccurred())
		})

		It("dumps all shares", func() {
			sharesChan := make(chan *collaboration.Share)
			receivedChan := make(chan share.ReceivedShareWithUser)

			shares := []*collaboration.Share{}

			wg := sync.WaitGroup{}
			wg.Add(2)
			go func() {
				for s := range sharesChan {
					if s != nil {
						shares = append(shares, s)
					}
				}
				wg.Done()
			}()
			go func() {
				for range receivedChan {
				}
				wg.Done()
			}()
			err := m.(share.DumpableManager).Dump(ctx, sharesChan, receivedChan)
			Expect(err).ToNot(HaveOccurred())
			close(sharesChan)
			close(receivedChan)
			wg.Wait()

			Expect(len(shares)).To(Equal(1))
			Expect(shares[0].Creator).To(Equal(user1.Id))
			Expect(shares[0].Grantee.GetUserId()).To(Equal(user2.Id))
			Expect(shares[0].ResourceId).To(Equal(sharedResource.Id))
		})

		It("dumps all received shares", func() {
			sharesChan := make(chan *collaboration.Share)
			receivedChan := make(chan share.ReceivedShareWithUser)

			shares := []share.ReceivedShareWithUser{}

			wg := sync.WaitGroup{}
			wg.Add(2)
			go func() {
				for range sharesChan {
				}
				wg.Done()
			}()
			go func() {
				for rs := range receivedChan {
					if rs.UserID != nil && rs.ReceivedShare != nil {
						shares = append(shares, rs)
					}
				}

				wg.Done()
			}()
			err := m.(share.DumpableManager).Dump(ctx, sharesChan, receivedChan)
			Expect(err).ToNot(HaveOccurred())
			close(sharesChan)
			close(receivedChan)
			wg.Wait()

			Expect(len(shares)).To(Equal(1))
			Expect(shares[0].UserID).To(Equal(user2.Id))
			Expect(shares[0].ReceivedShare.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))
			Expect(shares[0].ReceivedShare.MountPoint.Path).To(Equal("newPath/"))
			Expect(shares[0].ReceivedShare.Share.Creator).To(Equal(user1.Id))
			Expect(shares[0].ReceivedShare.Share.Grantee.GetUserId()).To(Equal(user2.Id))
			Expect(shares[0].ReceivedShare.Share.ResourceId).To(Equal(sharedResource.Id))
		})
	})
})
