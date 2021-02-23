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
	"path"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	helpers "github.com/cs3org/reva/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Grants", func() {
	var (
		env *helpers.TestEnv

		ref   *provider.Reference
		grant *provider.Grant
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: "/dir1",
			},
		}

		grant = &provider.Grant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id: &provider.Grantee_UserId{
					UserId: &userpb.UserId{
						OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
					},
				},
			},
			Permissions: &provider.ResourcePermissions{
				Stat:   true,
				Move:   true,
				Delete: false,
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

	Context("with insufficient permissions", func() {
		JustBeforeEach(func() {
			env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		})

		Describe("AddGrant", func() {
			It("adds grants", func() {
				err := env.Fs.AddGrant(env.Ctx, ref, grant)
				Expect(err).To(MatchError(ContainSubstring("permission denied")))
			})
		})
	})

	Context("with sufficient permissions", func() {
		JustBeforeEach(func() {
			env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
		})

		Describe("AddGrant", func() {
			It("adds grants", func() {
				n, err := env.Lookup.NodeFromPath(env.Ctx, "/dir1")
				Expect(err).ToNot(HaveOccurred())

				err = env.Fs.AddGrant(env.Ctx, ref, grant)
				Expect(err).ToNot(HaveOccurred())

				localPath := path.Join(env.Root, "nodes", n.ID)
				attr, err := xattr.Get(localPath, xattrs.GrantPrefix+xattrs.UserAcePrefix+grant.Grantee.GetUserId().OpaqueId)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(attr)).To(Equal("\x00t=A:f=:p=rw"))
			})
		})

		Describe("ListGrants", func() {
			It("lists existing grants", func() {
				err := env.Fs.AddGrant(env.Ctx, ref, grant)
				Expect(err).ToNot(HaveOccurred())

				grants, err := env.Fs.ListGrants(env.Ctx, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(grants)).To(Equal(1))

				g := grants[0]
				Expect(g.Grantee.GetUserId().OpaqueId).To(Equal(grant.Grantee.GetUserId().OpaqueId))
				Expect(g.Permissions.Stat).To(BeTrue())
				Expect(g.Permissions.Move).To(BeTrue())
				Expect(g.Permissions.Delete).To(BeFalse())
			})
		})

		Describe("RemoveGrants", func() {
			It("removes the grant", func() {
				err := env.Fs.AddGrant(env.Ctx, ref, grant)
				Expect(err).ToNot(HaveOccurred())

				grants, err := env.Fs.ListGrants(env.Ctx, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(grants)).To(Equal(1))

				err = env.Fs.RemoveGrant(env.Ctx, ref, grant)
				Expect(err).ToNot(HaveOccurred())

				grants, err = env.Fs.ListGrants(env.Ctx, ref)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(grants)).To(Equal(0))
			})
		})
	})
})
