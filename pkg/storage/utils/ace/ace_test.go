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

package ace_test

import (
	"fmt"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage/utils/ace"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ACE", func() {

	var (
		userGrant = &provider.Grant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id: &provider.Grantee_UserId{
					UserId: &userpb.UserId{
						OpaqueId: "foo",
					},
				},
			},
			Permissions: &provider.ResourcePermissions{},
		}

		groupGrant = &provider.Grant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id: &provider.Grantee_GroupId{
					GroupId: &grouppb.GroupId{
						OpaqueId: "foo",
					},
				},
			},
			Permissions: &provider.ResourcePermissions{},
		}
	)

	Describe("FromGrant", func() {
		It("creates an ACE from a user grant", func() {
			ace := ace.FromGrant(userGrant)
			Expect(ace.Principal()).To(Equal("u:foo"))
		})

		It("creates an ACE from a group grant", func() {
			ace := ace.FromGrant(groupGrant)
			Expect(ace.Principal()).To(Equal("g:foo"))
		})
	})

	Describe("Grant", func() {
		It("returns a proper Grant", func() {
			ace := ace.FromGrant(userGrant)
			grant := ace.Grant()
			Expect(grant).To(Equal(userGrant))
		})
	})

	Describe("marshalling", func() {
		It("works", func() {
			a := ace.FromGrant(userGrant)

			marshalled, principal := a.Marshal()
			unmarshalled, err := ace.Unmarshal(marshalled, principal)
			Expect(err).ToNot(HaveOccurred())

			Expect(unmarshalled).To(Equal(a))
		})
	})

	Describe("converting permissions", func() {
		It("converts r", func() {
			userGrant.Permissions.Stat = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.Stat = false
			Expect(newGrant.Permissions.Stat).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.ListContainer = true
			newGrant = ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.ListContainer = false
			Expect(newGrant.Permissions.ListContainer).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.InitiateFileDownload = true
			newGrant = ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.InitiateFileDownload = false
			Expect(newGrant.Permissions.InitiateFileDownload).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.GetPath = true
			newGrant = ace.FromGrant(userGrant).Grant()
			fmt.Println(newGrant.Permissions)
			userGrant.Permissions.GetPath = false
			Expect(newGrant.Permissions.GetPath).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts w", func() {
			userGrant.Permissions.InitiateFileUpload = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.InitiateFileUpload = false
			Expect(newGrant.Permissions.InitiateFileUpload).To(BeTrue())
			Expect(newGrant.Permissions.Move).To(BeFalse())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.InitiateFileUpload = true
			userGrant.Permissions.InitiateFileDownload = true
			newGrant = ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.InitiateFileUpload = false
			Expect(newGrant.Permissions.InitiateFileUpload).To(BeTrue())
			Expect(newGrant.Permissions.Move).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts a", func() {
			userGrant.Permissions.CreateContainer = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.CreateContainer = false
			Expect(newGrant.Permissions.CreateContainer).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts d", func() {
			userGrant.Permissions.Delete = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.Delete = false
			Expect(newGrant.Permissions.Delete).To(BeTrue())
			Expect(newGrant.Permissions.Move).To(BeFalse())
		})

		It("converts C", func() {
			userGrant.Permissions.AddGrant = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.AddGrant = false
			Expect(newGrant.Permissions.AddGrant).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.RemoveGrant = true
			newGrant = ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.RemoveGrant = false
			Expect(newGrant.Permissions.RemoveGrant).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())

			userGrant.Permissions.UpdateGrant = true
			newGrant = ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.UpdateGrant = false
			Expect(newGrant.Permissions.UpdateGrant).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts c", func() {
			userGrant.Permissions.ListGrants = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.ListGrants = false
			Expect(newGrant.Permissions.ListGrants).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts u", func() {
			userGrant.Permissions.ListRecycle = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.ListRecycle = false
			Expect(newGrant.Permissions.ListRecycle).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts U", func() {
			userGrant.Permissions.RestoreRecycleItem = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.RestoreRecycleItem = false
			Expect(newGrant.Permissions.RestoreRecycleItem).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts P", func() {
			userGrant.Permissions.PurgeRecycle = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.PurgeRecycle = false
			Expect(newGrant.Permissions.PurgeRecycle).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts v", func() {
			userGrant.Permissions.ListFileVersions = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.ListFileVersions = false
			Expect(newGrant.Permissions.ListFileVersions).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts V", func() {
			userGrant.Permissions.RestoreFileVersion = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.RestoreFileVersion = false
			Expect(newGrant.Permissions.RestoreFileVersion).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})

		It("converts q", func() {
			userGrant.Permissions.GetQuota = true
			newGrant := ace.FromGrant(userGrant).Grant()
			userGrant.Permissions.GetQuota = false
			Expect(newGrant.Permissions.GetQuota).To(BeTrue())
			Expect(newGrant.Permissions.Delete).To(BeFalse())
		})
	})
})
