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

package node_test

import (
	"context"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
)

var _ = Describe("Node locks", func() {
	var (
		env *helpers.TestEnv

		lock      *provider.Lock
		wrongLock *provider.Lock
		n         *node.Node
		id        string
		name      string
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv()
		Expect(err).ToNot(HaveOccurred())

		lock = &provider.Lock{
			Type:   provider.LockType_LOCK_TYPE_EXCL,
			User:   env.Owner.Id,
			LockId: uuid.New().String(),
		}
		wrongLock = &provider.Lock{
			Type:   provider.LockType_LOCK_TYPE_EXCL,
			User:   env.Owner.Id,
			LockId: uuid.New().String(),
		}
		id = "fooId"
		name = "foo"
		n = node.New(id, "", name, 10, "", env.Owner.Id, env.Lookup)
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("SetLock", func() {
		It("sets the lock", func() {
			_, err := os.Stat(n.LockFilePath())
			Expect(err).To(HaveOccurred())

			err = n.SetLock(env.Ctx, lock)
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Stat(n.LockFilePath())
			Expect(err).ToNot(HaveOccurred())
		})

		It("refuses to lock if already locked an existing lock was not provided", func() {
			err := n.SetLock(env.Ctx, lock)
			Expect(err).ToNot(HaveOccurred())

			err = n.SetLock(env.Ctx, lock)
			Expect(err).To(HaveOccurred())

			env.Ctx = ctxpkg.ContextSetLockID(env.Ctx, wrongLock.LockId)
			err = n.SetLock(env.Ctx, lock)
			Expect(err).To(HaveOccurred())
		})

		It("relocks if the existing lock was provided", func() {
			err := n.SetLock(env.Ctx, lock)
			Expect(err).ToNot(HaveOccurred())

			env.Ctx = ctxpkg.ContextSetLockID(env.Ctx, lock.LockId)
			err = n.SetLock(env.Ctx, lock)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with an existing lock", func() {
		BeforeEach(func() {
			err := n.SetLock(env.Ctx, lock)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("Unlock", func() {
			It("refuses to unlock without having a lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lock.LockId))
			})

			It("refuses to unlock without having the proper lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lock.LockId))

				err = n.Unlock(env.Ctx, wrongLock)
				Expect(err.Error()).To(ContainSubstring(lock.LockId))
			})

			It("refuses to unlock for others even if they have the lock", func() {
				otherUser := &userpb.User{
					Id: &userpb.UserId{
						Idp:      "idp",
						OpaqueId: "foo",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
					Username: "foo",
				}
				otherCtx := ctxpkg.ContextSetUser(context.Background(), otherUser)

				err := n.Unlock(otherCtx, lock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("mismatching"))
			})

			It("unlocks when the owner uses the lock", func() {
				err := n.Unlock(env.Ctx, lock)
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(n.LockFilePath())
				Expect(err).To(HaveOccurred())
			})

			It("fails to unlock an unlocked node", func() {
				err := n.Unlock(env.Ctx, lock)
				Expect(err).ToNot(HaveOccurred())

				err = n.Unlock(env.Ctx, lock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("lock does not exist"))
			})
		})
	})
})
