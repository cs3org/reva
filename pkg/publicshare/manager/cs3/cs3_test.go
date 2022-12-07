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
	"os"
	"path"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/cs3"
	indexerpkg "github.com/cs3org/reva/v2/pkg/storage/utils/indexer"
	indexermocks "github.com/cs3org/reva/v2/pkg/storage/utils/indexer/mocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	storagemocks "github.com/cs3org/reva/v2/pkg/storage/utils/metadata/mocks"
	"github.com/cs3org/reva/v2/pkg/utils"
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
		tmpdir, err = os.MkdirTemp("", "cs3-publicshare-test")
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

	JustBeforeEach(func() {
		var err error
		m, err = cs3.New(nil, storage, indexer, bcrypt.DefaultCost)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			m, err := cs3.New(nil, storage, indexer, bcrypt.DefaultCost)
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
				ps := publicshare.WithPassword{}
				err = json.Unmarshal(in, &ps)
				Expect(err).ToNot(HaveOccurred())
				return bcrypt.CompareHashAndPassword([]byte(ps.Password), []byte("secret123")) == nil
			}))
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

			h, err := bcrypt.GenerateFromPassword([]byte(grant.Password), bcrypt.DefaultCost)
			Expect(err).ToNot(HaveOccurred())
			hashedPassword = string(h)
			shareJSON, err := json.Marshal(publicshare.WithPassword{PublicShare: *existingShare, Password: hashedPassword})
			Expect(err).ToNot(HaveOccurred())
			storage.On("SimpleDownload", mock.Anything, mock.MatchedBy(func(in string) bool {
				return strings.HasPrefix(in, "publicshares/")
			})).Return(shareJSON, nil)
		})

		Describe("Load", func() {
			It("loads shares including state and mountpoint information", func() {
				m, err := cs3.New(nil, storage, indexer, bcrypt.DefaultCost)
				Expect(err).ToNot(HaveOccurred())

				sharesChan := make(chan *publicshare.WithPassword)

				wg := sync.WaitGroup{}
				wg.Add(2)
				go func() {
					err := m.Load(ctx, sharesChan)
					Expect(err).ToNot(HaveOccurred())
					wg.Done()
				}()
				go func() {
					sharesChan <- &publicshare.WithPassword{
						Password:    "foo",
						PublicShare: *existingShare,
					}
					close(sharesChan)
					wg.Done()
				}()
				wg.Wait()
				Eventually(sharesChan).Should(BeClosed())

				expectedPath := path.Join("publicshares", existingShare.Token)
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, expectedPath, mock.Anything)
			})
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
					storage.On("Delete", mock.Anything, mock.Anything).Return(nil, nil)
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
						Token: share.Token,
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
				mockIndexer *indexermocks.Indexer
			)
			BeforeEach(func() {
				mockIndexer = &indexermocks.Indexer{}
				mockIndexer.On("AddIndex", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockIndexer.On("Add", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
				mockIndexer.On("Delete", mock.Anything, mock.Anything).Return(nil, nil)
				mockIndexer.On("FindBy", mock.Anything, mock.Anything, mock.Anything).Return([]string{existingShare.Token}, nil)

				indexer = mockIndexer
			})

			It("deletes the share by token", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(nil)
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).ToNot(HaveOccurred())
				storage.AssertCalled(GinkgoT(), "Delete", mock.Anything, path.Join("publicshares", existingShare.Token))
			})

			It("deletes the share by id", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(nil)
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: existingShare.Id,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).ToNot(HaveOccurred())
				storage.AssertCalled(GinkgoT(), "Delete", mock.Anything, path.Join("publicshares", existingShare.Token))
			})

			It("still removes the share from the index when the share itself couldn't be found", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(errtypes.NotFound(""))
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).ToNot(HaveOccurred())

				mockIndexer.AssertCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
			})

			It("does not removes the share from the index when the share itself couldn't be found", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(errtypes.InternalError(""))
				ref := &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}
				err := m.RevokePublicShare(ctx, user, ref)
				Expect(err).To(HaveOccurred())

				mockIndexer.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
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
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, path.Join("publicshares", ps.Token), mock.MatchedBy(func(data []byte) bool {
					s := publicshare.WithPassword{}
					err := json.Unmarshal(data, &s)
					Expect(err).ToNot(HaveOccurred())
					return s.PublicShare.DisplayName == "new displayname"
				}))
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
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, path.Join("publicshares", ps.Token), mock.MatchedBy(func(data []byte) bool {
					s := publicshare.WithPassword{}
					err := json.Unmarshal(data, &s)
					Expect(err).ToNot(HaveOccurred())
					return s.Password != ""
				}))
			})

			It("updates the permissions", func() {
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: &provider.ResourcePermissions{Delete: true}}},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, path.Join("publicshares", ps.Token), mock.MatchedBy(func(data []byte) bool {
					s := publicshare.WithPassword{}
					err := json.Unmarshal(data, &s)
					Expect(err).ToNot(HaveOccurred())
					return s.PublicShare.Permissions.Permissions.Delete
				}))
			})

			It("updates the expiration", func() {
				ts := utils.TSNow()
				ps, err := m.UpdatePublicShare(ctx, user, &link.UpdatePublicShareRequest{
					Ref: ref,
					Update: &link.UpdatePublicShareRequest_Update{
						Type:  link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
						Grant: &link.Grant{Expiration: utils.TSNow()},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, path.Join("publicshares", ps.Token), mock.MatchedBy(func(data []byte) bool {
					s := publicshare.WithPassword{}
					err := json.Unmarshal(data, &s)
					Expect(err).ToNot(HaveOccurred())
					return s.PublicShare.Expiration != nil && s.PublicShare.Expiration.Seconds == ts.Seconds
				}))
			})
		})
	})
})
