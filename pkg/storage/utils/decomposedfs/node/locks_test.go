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

		lock *provider.Lock
		n    *node.Node
		id   string
		name string
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
				wrongLock := &provider.Lock{
					Type:   provider.LockType_LOCK_TYPE_EXCL,
					User:   env.Owner.Id,
					LockId: uuid.New().String(),
				}
				err := n.Unlock(env.Ctx, wrongLock)
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
