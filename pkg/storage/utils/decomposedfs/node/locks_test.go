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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
)

var _ = Describe("Node locks", func() {
	var (
		env *helpers.TestEnv

		lockByUser      *provider.Lock
		wrongLockByUser *provider.Lock
		lockByApp       *provider.Lock
		wrongLockByApp  *provider.Lock
		n               *node.Node
		n2              *node.Node

		otherUser = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "otheruserid",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "foo",
		}
		otherCtx = ctxpkg.ContextSetUser(context.Background(), otherUser)
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		lockByUser = &provider.Lock{
			Type:   provider.LockType_LOCK_TYPE_EXCL,
			User:   env.Owner.Id,
			LockId: uuid.New().String(),
		}
		wrongLockByUser = &provider.Lock{
			Type:   provider.LockType_LOCK_TYPE_EXCL,
			User:   env.Owner.Id,
			LockId: uuid.New().String(),
		}
		lockByApp = &provider.Lock{
			Type:    provider.LockType_LOCK_TYPE_WRITE,
			AppName: "app1",
			LockId:  uuid.New().String(),
		}
		wrongLockByApp = &provider.Lock{
			Type:    provider.LockType_LOCK_TYPE_WRITE,
			AppName: "app2",
			LockId:  uuid.New().String(),
		}
		n = node.New("u-s-e-r-id", "tobelockedid", "", "tobelocked", 10, "", env.Owner.Id, env.Lookup)
		n2 = node.New("u-s-e-r-id", "neverlockedid", "", "neverlocked", 10, "", env.Owner.Id, env.Lookup)
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	Describe("SetLock for a user", func() {
		It("sets the lock", func() {
			_, err := os.Stat(n.LockFilePath())
			Expect(err).To(HaveOccurred())

			err = n.SetLock(env.Ctx, lockByUser)
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Stat(n.LockFilePath())
			Expect(err).ToNot(HaveOccurred())
		})

		It("refuses to set a lock if already locked", func() {
			err := n.SetLock(env.Ctx, lockByUser)
			Expect(err).ToNot(HaveOccurred())

			err = n.SetLock(env.Ctx, lockByUser)
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.PreconditionFailed)
			Expect(ok).To(BeTrue())
		})
	})

	Describe("SetLock for an app", func() {
		It("sets the lock", func() {
			_, err := os.Stat(n.LockFilePath())
			Expect(err).To(HaveOccurred())

			err = n.SetLock(env.Ctx, lockByApp)
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Stat(n.LockFilePath())
			Expect(err).ToNot(HaveOccurred())
		})

		It("refuses to set a lock if already locked", func() {
			err := n.SetLock(env.Ctx, lockByApp)
			Expect(err).ToNot(HaveOccurred())

			err = n.SetLock(env.Ctx, lockByApp)
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.PreconditionFailed)
			Expect(ok).To(BeTrue())
		})

	})

	Context("with an existing lock for a user", func() {
		BeforeEach(func() {
			err := n.SetLock(env.Ctx, lockByUser)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("ReadLock", func() {
			It("returns the lock", func() {
				l, err := n.ReadLock(env.Ctx, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(l).To(Equal(lockByUser))
			})

			It("reports an error when the node wasn't locked", func() {
				_, err := n2.ReadLock(env.Ctx, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no lock found"))
			})
		})

		Describe("RefreshLock", func() {
			var (
				newLock *provider.Lock
			)

			JustBeforeEach(func() {
				newLock = &provider.Lock{
					Type:   provider.LockType_LOCK_TYPE_EXCL,
					User:   env.Owner.Id,
					LockId: lockByUser.LockId,
				}
			})

			It("fails when the node is unlocked", func() {
				err := n2.RefreshLock(env.Ctx, lockByUser)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("precondition failed"))
			})

			It("refuses to refresh the lock without holding the lock", func() {
				newLock.LockId = "somethingsomething"
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("mismatching"))
			})

			It("refuses to refresh the lock for other users than the lock holder", func() {
				err := n.RefreshLock(otherCtx, newLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("refuses to change the lock holder", func() {
				newLock.User = otherUser.Id
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("refreshes the lock", func() {
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("Unlock", func() {
			It("refuses to unlock without having a lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lockByUser.LockId))
			})

			It("refuses to unlock without having the proper lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lockByUser.LockId))

				err = n.Unlock(env.Ctx, wrongLockByUser)
				Expect(err.Error()).To(ContainSubstring(lockByUser.LockId))
			})

			It("refuses to unlock for others even if they have the lock", func() {
				err := n.Unlock(otherCtx, lockByUser)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("mismatching"))
			})

			It("unlocks when the owner uses the lock", func() {
				err := n.Unlock(env.Ctx, lockByUser)
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(n.LockFilePath())
				Expect(err).To(HaveOccurred())
			})

			It("fails to unlock an unlocked node", func() {
				err := n.Unlock(env.Ctx, lockByUser)
				Expect(err).ToNot(HaveOccurred())

				err = n.Unlock(env.Ctx, lockByUser)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("lock does not exist"))
			})
		})
	})

	Context("with an existing lock for an app", func() {
		BeforeEach(func() {
			err := n.SetLock(env.Ctx, lockByApp)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("ReadLock", func() {
			It("returns the lock", func() {
				l, err := n.ReadLock(env.Ctx, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(l).To(Equal(lockByApp))
			})

			It("reports an error when the node wasn't locked", func() {
				_, err := n2.ReadLock(env.Ctx, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no lock found"))
			})
		})

		Describe("RefreshLock", func() {
			var (
				newLock *provider.Lock
			)

			JustBeforeEach(func() {
				newLock = &provider.Lock{
					Type:    provider.LockType_LOCK_TYPE_EXCL,
					AppName: lockByApp.AppName,
					LockId:  lockByApp.LockId,
				}
			})

			It("fails when the node is unlocked", func() {
				err := n2.RefreshLock(env.Ctx, lockByApp)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("precondition failed"))
			})

			It("refuses to refresh the lock without holding the lock", func() {
				newLock.LockId = "somethingsomething"
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("mismatching"))
			})

			It("refreshes the lock for other users", func() {
				err := n.RefreshLock(otherCtx, lockByApp)
				Expect(err).ToNot(HaveOccurred())
			})

			It("refuses to change the lock holder", func() {
				newLock.AppName = wrongLockByApp.AppName
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("refreshes the lock", func() {
				err := n.RefreshLock(env.Ctx, newLock)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("Unlock", func() {
			It("refuses to unlock without having a lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lockByApp.LockId))
			})

			It("refuses to unlock without having the proper lock", func() {
				err := n.Unlock(env.Ctx, nil)
				Expect(err.Error()).To(ContainSubstring(lockByApp.LockId))

				err = n.Unlock(env.Ctx, wrongLockByUser)
				Expect(err.Error()).To(ContainSubstring(lockByApp.LockId))
			})

			It("accepts to unlock for others if they have the lock", func() {
				err := n.Unlock(otherCtx, lockByApp)
				Expect(err).ToNot(HaveOccurred())
			})

			It("unlocks when the owner uses the lock", func() {
				err := n.Unlock(env.Ctx, lockByApp)
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(n.LockFilePath())
				Expect(err).To(HaveOccurred())
			})

			It("fails to unlock an unlocked node", func() {
				err := n.Unlock(env.Ctx, lockByApp)
				Expect(err).ToNot(HaveOccurred())

				err = n.Unlock(env.Ctx, lockByApp)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("lock does not exist"))
			})
		})
	})
})
