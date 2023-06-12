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

package receivedsharecache_test

import (
	"context"
	"os"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/receivedsharecache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c       receivedsharecache.Cache
		storage metadata.Storage

		userID  = "user"
		spaceID = "spaceid"
		shareID = "storageid$spaceid!share1"
		share   = &collaboration.Share{
			Id: &collaborationv1beta1.ShareId{
				OpaqueId: shareID,
			},
		}
		ctx    context.Context
		tmpdir string
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		tmpdir, err = os.MkdirTemp("", "providercache-test")
		Expect(err).ToNot(HaveOccurred())

		err = os.MkdirAll(tmpdir, 0755)
		Expect(err).ToNot(HaveOccurred())

		storage, err = metadata.NewDiskStorage(tmpdir)
		Expect(err).ToNot(HaveOccurred())

		c = receivedsharecache.New(storage, 0*time.Second)
		Expect(c).ToNot(BeNil()) // nolint:copylocks
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("Add", func() {
		It("adds an entry", func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaborationv1beta1.ShareState_SHARE_STATE_PENDING,
			}
			err := c.Add(ctx, userID, spaceID, rs)
			Expect(err).ToNot(HaveOccurred())

			s := c.Get(userID, spaceID, shareID)
			Expect(s).ToNot(BeNil())
		})

		It("persists the new entry", func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaborationv1beta1.ShareState_SHARE_STATE_PENDING,
			}
			err := c.Add(ctx, userID, spaceID, rs)
			Expect(err).ToNot(HaveOccurred())

			c = receivedsharecache.New(storage, 0*time.Second)
			Expect(c.Sync(ctx, userID)).To(Succeed())
			s := c.Get(userID, spaceID, shareID)
			Expect(s).ToNot(BeNil())
		})
	})

	Describe("with an existing entry", func() {
		BeforeEach(func() {
			rs := &collaboration.ReceivedShare{
				Share: share,
				State: collaborationv1beta1.ShareState_SHARE_STATE_PENDING,
			}
			Expect(c.Add(ctx, userID, spaceID, rs)).To(Succeed())
		})

		Describe("Get", func() {
			It("handles unknown users", func() {
				s := c.Get("something", spaceID, shareID)
				Expect(s).To(BeNil())
			})

			It("handles unknown spaces", func() {
				s := c.Get(userID, "something", shareID)
				Expect(s).To(BeNil())
			})

			It("handles unknown shares", func() {
				s := c.Get(userID, spaceID, "something")
				Expect(s).To(BeNil())
			})

			It("gets the entry", func() {
				s := c.Get(userID, spaceID, shareID)
				Expect(s).ToNot(BeNil())
			})
		})
	})
})
