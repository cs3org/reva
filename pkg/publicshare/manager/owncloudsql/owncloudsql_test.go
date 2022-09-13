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

package owncloudsql_test

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/owncloudsql"
	"github.com/cs3org/reva/v2/pkg/share/manager/owncloudsql/mocks"
	"github.com/stretchr/testify/mock"

	_ "github.com/mattn/go-sqlite3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SQL manager", func() {
	var (
		m    publicshare.Manager
		user *userpb.User
		ctx  context.Context

		ri    *provider.ResourceInfo
		grant *link.Grant
		//share *link.PublicShare

		testDbFile *os.File
		sqldb      *sql.DB
	)

	AfterEach(func() {
		os.Remove(testDbFile.Name())
	})

	BeforeEach(func() {
		var err error
		testDbFile, err = ioutil.TempFile("", "example")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := ioutil.ReadFile("test.db")
		Expect(err).ToNot(HaveOccurred())

		_, err = testDbFile.Write(dbData)
		Expect(err).ToNot(HaveOccurred())
		err = testDbFile.Close()
		Expect(err).ToNot(HaveOccurred())

		sqldb, err = sql.Open("sqlite3", testDbFile.Name())
		Expect(err).ToNot(HaveOccurred())

		userConverter := &mocks.UserConverter{}
		userConverter.On("UserIDToUserName", mock.Anything, mock.Anything).Return("username", nil)
		userConverter.On("UserNameToUserID", mock.Anything, mock.Anything).Return(
			func(_ context.Context, username string) *userpb.UserId {
				return &userpb.UserId{
					OpaqueId: username,
				}
			},
			func(_ context.Context, username string) error { return nil })
		m, err = owncloudsql.New("sqlite3", sqldb, owncloudsql.Config{}, userConverter)
		Expect(err).ToNot(HaveOccurred())

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "localhost:1111",
				OpaqueId: "1",
			},
			Username: "username",
		}
		ctx = ctxpkg.ContextSetUser(context.Background(), user)

		/*
			share = &link.PublicShare{
				Id:    &link.PublicShareId{OpaqueId: "1"},
				Token: "abcd",
			}
		*/

		ri = &provider.ResourceInfo{
			Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			Path:  "/share1",
			Id:    &provider.ResourceId{OpaqueId: "14"}, // matches the fileid in the oc_filecache
			Owner: user.Id,
			PermissionSet: &provider.ResourcePermissions{
				Stat: true,
			},
			Size: 10,
		}
		grant = &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: &provider.ResourcePermissions{AddGrant: true},
			},
		}
	})

	It("creates manager instances", func() {
		Expect(m).ToNot(BeNil())
	})
	Describe("CreatePublicShare", func() {
		It("creates a new share and adds it to the index", func() {
			link, err := m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(link).ToNot(BeNil())
			Expect(link.Token).ToNot(Equal(""))
			Expect(link.PasswordProtected).To(BeFalse())
		})

		It("sets 'PasswordProtected' and stores the password hash if a password is set", func() {
			grant.Password = "secret123"

			link, err := m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(link).ToNot(BeNil())
			Expect(link.Token).ToNot(Equal(""))
			Expect(link.PasswordProtected).To(BeTrue())
			// TODO check it is in the db?
		})

		It("picks up the displayname from the metadata", func() {
			ri.ArbitraryMetadata = &provider.ArbitraryMetadata{
				Metadata: map[string]string{
					"name": "metadata name",
				},
			}

			link, err := m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(link).ToNot(BeNil())
			Expect(link.DisplayName).To(Equal("metadata name"))
		})
	})

	Context("with an existing share", func() {

		JustBeforeEach(func() {
			grant.Password = "foo"
			var existingShare *link.PublicShare
			var err error
			existingShare, err = m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(existingShare).ToNot(BeNil())
		})

		Describe("ListPublicShares", func() {
			It("lists existing shares", func() {
				shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
				Expect(shares[0].Signature).To(BeNil())
			})
		})
	})
})
