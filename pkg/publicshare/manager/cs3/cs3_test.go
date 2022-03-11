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

package cs3_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/cs3"
	indexerpkg "github.com/cs3org/reva/v2/pkg/storage/utils/indexer"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	storagemocks "github.com/cs3org/reva/v2/pkg/storage/utils/metadata/mocks"
)

var _ = Describe("Cs3", func() {
	var (
		m    publicshare.Manager
		user *userpb.User
		ctx  context.Context

		indexer indexerpkg.Indexer
		storage *storagemocks.Storage

		ri    *provider.ResourceInfo
		grant *link.Grant
		share *link.PublicShare

		tmpdir string
	)

	BeforeEach(func() {
		var err error
		tmpdir, err = ioutil.TempDir("", "cs3-publicshare-test")
		Expect(err).ToNot(HaveOccurred())

		ds, err := metadata.NewDiskStorage(tmpdir)
		Expect(err).ToNot(HaveOccurred())
		indexer = indexerpkg.CreateIndexer(ds)

		storage = &storagemocks.Storage{}
		storage.On("Init", mock.Anything, mock.Anything).Return(nil)
		storage.On("MakeDirIfNotExist", mock.Anything, mock.Anything).Return(nil)
		storage.On("SimpleUpload", mock.Anything, mock.MatchedBy(func(in string) bool {
			return strings.HasPrefix(in, "publicshares/")
		}), mock.Anything).Return(nil)
		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "localhost:1111",
				OpaqueId: "1",
			},
		}
		ctx = ctxpkg.ContextSetUser(context.Background(), user)

		m, err = cs3.New(storage, indexer, bcrypt.DefaultCost)
		Expect(err).ToNot(HaveOccurred())

		share = &link.PublicShare{
			Id:    &link.PublicShareId{OpaqueId: "1"},
			Token: "abcd",
		}

		ri = &provider.ResourceInfo{
			Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			Path:  "/share1",
			Id:    &provider.ResourceId{OpaqueId: "sharedId"},
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

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			m, err := cs3.New(storage, indexer, bcrypt.DefaultCost)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).ToNot(BeNil())
		})
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
			storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, mock.Anything, mock.MatchedBy(func(in []byte) bool {
				ps := cs3.PublicShareWithPassword{}
				err = json.Unmarshal(in, &ps)
				Expect(err).ToNot(HaveOccurred())
				return bcrypt.CompareHashAndPassword([]byte(ps.HashedPassword), []byte("secret123")) == nil
			}))
		})
	})

	Context("with an existing share", func() {
		var (
			existingShare *link.PublicShare
		)

		BeforeEach(func() {
			var err error
			existingShare, err = m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("GetPublicShare", func() {
			var (
				shareJson []byte
			)

			BeforeEach(func() {
				var err error
				shareJson, err = json.Marshal(share)
				Expect(err).ToNot(HaveOccurred())
				storage.On("SimpleDownload", mock.Anything, mock.MatchedBy(func(in string) bool {
					return strings.HasPrefix(in, "publicshares/")
				})).Return(shareJson, nil)
			})

			It("gets the public share by token", func() {
				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: share.Token,
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.Id.OpaqueId).To(Equal(share.Id.OpaqueId))
				Expect(returnedShare.Token).To(Equal(share.Token))
			})

			It("gets the public share by id", func() {
				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: existingShare.Id.OpaqueId,
						},
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
			})
		})
	})
})
