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
	"os"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/v2/internal/http/services/owncloud/ocs/conversions"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/owncloudsql"
	"github.com/cs3org/reva/v2/pkg/share/manager/owncloudsql/mocks"
	"github.com/cs3org/reva/v2/pkg/utils"
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
		// share *link.PublicShare

		testDbFile *os.File
		sqldb      *sql.DB
	)

	AfterEach(func() {
		os.Remove(testDbFile.Name())
	})

	BeforeEach(func() {
		var err error
		testDbFile, err = os.CreateTemp("", "example")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := os.ReadFile("test.db")
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
				Permissions: conversions.NewViewerRole().CS3ResourcePermissions(),
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
		var (
			existingShare  *link.PublicShare
			hashedPassword string
		)

		JustBeforeEach(func() {
			grant.Password = "foo"
			var err error
			existingShare, err = m.CreatePublicShare(ctx, user, ri, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(existingShare).ToNot(BeNil())

			// read hashed password from db
			s, err := owncloudsql.GetByToken(sqldb, existingShare.Token)
			Expect(err).ToNot(HaveOccurred())
			hashedPassword = strings.TrimPrefix(s.ShareWith, "1|")
		})

		Describe("ListPublicShares", func() {
			It("lists existing shares", func() {
				shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
				Expect(shares[0].Signature).To(BeNil())
			})

			It("adds a signature", func() {
				shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{}, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
				Expect(shares[0].Signature).ToNot(BeNil())
			})

			It("filters by id", func() {
				shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{
					publicshare.ResourceIDFilter(&provider.ResourceId{OpaqueId: "UnknownId"}),
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(0))
			})

			It("filters by storage", func() {
				shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{
					publicshare.StorageIDFilter("unknownstorage"),
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(0))
			})

			Context("when the share has expired", func() {
				BeforeEach(func() {
					t := time.Date(2022, time.January, 1, 12, 0, 0, 0, time.UTC)
					grant.Expiration = &typespb.Timestamp{
						Seconds: uint64(t.Unix()),
					}
				})

				It("does not consider the share", func() {
					shares, err := m.ListPublicShares(ctx, user, []*link.ListPublicSharesRequest_Filter{}, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(shares)).To(Equal(0))
				})
			})
		})

		Describe("GetPublicShare", func() {
			It("gets the public share by token", func() {
				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.Id.OpaqueId).To(Equal(existingShare.Id.OpaqueId))
				Expect(returnedShare.Token).To(Equal(existingShare.Token))
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
				Expect(returnedShare.Signature).To(BeNil())
			})

			It("adds a signature", func() {
				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: existingShare.Id.OpaqueId,
						},
					},
				}, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.Signature).ToNot(BeNil())
			})
		})

		Describe("RevokePublicShare", func() {
			var (
				existingShare *link.PublicShare
			)
			BeforeEach(func() {
				grant.Password = "foo"
				var err error
				existingShare, err = m.CreatePublicShare(ctx, user, ri, grant)
				Expect(err).ToNot(HaveOccurred())
				Expect(existingShare).ToNot(BeNil())
			})

			It("deletes the share by token", func() {
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).ToNot(HaveOccurred())

				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: existingShare.Id.OpaqueId,
						},
					},
				}, false)
				Expect(err).To(HaveOccurred())
				Expect(returnedShare).To(BeNil())
			})

			It("deletes the share by id", func() {
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: existingShare.Id,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).ToNot(HaveOccurred())

				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: existingShare.Id.OpaqueId,
						},
					},
				}, false)
				Expect(err).To(HaveOccurred())
				Expect(returnedShare).To(BeNil())
			})
		})

		Describe("GetPublicShareByToken", func() {
			It("doesn't get the share using a wrong password", func() {
				auth := &link.PublicShareAuthentication{
					Spec: &link.PublicShareAuthentication_Password{
						Password: "wroooong",
					},
				}
				ps, err := m.GetPublicShareByToken(ctx, existingShare.Token, auth, false)
				Expect(err).To(HaveOccurred())
				Expect(ps).To(BeNil())
			})

			It("gets the share using a password", func() {
				auth := &link.PublicShareAuthentication{
					Spec: &link.PublicShareAuthentication_Password{
						Password: grant.Password,
					},
				}
				ps, err := m.GetPublicShareByToken(ctx, existingShare.Token, auth, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
			})

			It("gets the share using a signature", func() {
				err := publicshare.AddSignature(existingShare, hashedPassword)
				Expect(err).ToNot(HaveOccurred())
				auth := &link.PublicShareAuthentication{
					Spec: &link.PublicShareAuthentication_Signature{
						Signature: existingShare.Signature,
					},
				}
				ps, err := m.GetPublicShareByToken(ctx, existingShare.Token, auth, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())

			})

			Context("when the share has expired", func() {
				BeforeEach(func() {
					t := time.Date(2022, time.January, 1, 12, 0, 0, 0, time.UTC)
					grant.Expiration = &typespb.Timestamp{
						Seconds: uint64(t.Unix()),
					}
				})
				It("it doesn't consider expired shares", func() {
					auth := &link.PublicShareAuthentication{
						Spec: &link.PublicShareAuthentication_Password{
							Password: grant.Password,
						},
					}
					ps, err := m.GetPublicShareByToken(ctx, existingShare.Token, auth, false)
					Expect(err).To(HaveOccurred())
					Expect(ps).To(BeNil())
				})
			})
		})

		Describe("UpdatePublicShare", func() {
			var (
				ref *link.PublicShareReference
			)

			JustBeforeEach(func() {
				ref = &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}
			})

			It("fails when an invalid reference is given", func() {
				_, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: "doesnotexist"}}},
				})
				Expect(err).To(HaveOccurred())
			})

			It("fails when no valid update request is given", func() {
				_, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref:    ref,
					Update: &link.UpdatePublicShareRequest_Update{},
				})
				Expect(err).To(HaveOccurred())
			})

			It("updates the display name", func() {
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
						DisplayName: "new displayname",
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
				Expect(ps.DisplayName).To(Equal("new displayname"))

				returnedShare, err := m.GetPublicShare(ctx, user, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: existingShare.Id.OpaqueId,
						},
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.DisplayName).To(Equal("new displayname"))
			})

			It("updates the password", func() {
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:  link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
						Grant: &link.Grant{Password: "NewPass"},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())

				auth := &link.PublicShareAuthentication{
					Spec: &link.PublicShareAuthentication_Password{
						Password: "NewPass",
					},
				}
				ps, err = m.GetPublicShareByToken(ctx, existingShare.Token, auth, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
			})

			It("updates the permissions", func() {
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: conversions.NewEditorRole().CS3ResourcePermissions()}},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())

				returnedShare, err := m.GetPublicShare(ctx, user, ref, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.Permissions.Permissions.Delete).To(BeTrue())
			})

			It("updates the expiration", func() {
				ts := utils.TimeToTS(time.Now().Add(1 * time.Hour))
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:  link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
						Grant: &link.Grant{Expiration: ts},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())

				returnedShare, err := m.GetPublicShare(ctx, user, ref, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(returnedShare).ToNot(BeNil())
				Expect(returnedShare.Expiration.Seconds).To(Equal(ts.Seconds))

			})
		})
	})
})
