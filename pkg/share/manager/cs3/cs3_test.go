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
	"net/url"
	"path"
	"sync"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	sharespkg "github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/share/manager/cs3"
	indexerpkg "github.com/cs3org/reva/v2/pkg/storage/utils/indexer"
	indexermocks "github.com/cs3org/reva/v2/pkg/storage/utils/indexer/mocks"
	storagemocks "github.com/cs3org/reva/v2/pkg/storage/utils/metadata/mocks"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	var (
		storage    *storagemocks.Storage
		indexer    *indexermocks.Indexer
		user       *userpb.User
		grantee    *userpb.User
		share      *collaboration.Share
		share2     *collaboration.Share
		groupShare *collaboration.Share
		grant      *collaboration.ShareGrant
		ctx        context.Context
		granteeCtx context.Context

		granteeFn string
		groupFn   string
	)

	BeforeEach(func() {
		storage = &storagemocks.Storage{}
		storage.On("Init", mock.Anything, mock.Anything).Return(nil)
		storage.On("MakeDirIfNotExist", mock.Anything, mock.Anything).Return(nil)
		storage.On("SimpleUpload", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		indexer = &indexermocks.Indexer{}
		indexer.On("AddIndex", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		indexer.On("Add", mock.Anything).Return([]indexerpkg.IdxAddResult{}, nil)
		indexer.On("Delete", mock.Anything).Return(nil)

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "localhost:1111",
				OpaqueId: "1",
			},
		}
		grantee = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "localhost:1111",
				OpaqueId: "2",
			},
			Groups: []string{"users"},
		}
		granteeFn = url.QueryEscape("user:" + grantee.Id.Idp + ":" + grantee.Id.OpaqueId)
		groupFn = url.QueryEscape("group:users")

		grant = &collaboration.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: grantee.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &provider.ResourcePermissions{
					GetPath:              true,
					InitiateFileDownload: true,
					ListFileVersions:     true,
					ListContainer:        true,
					Stat:                 true,
				},
			},
		}
		share = &collaboration.Share{
			Id:         &collaboration.ShareId{OpaqueId: "1"},
			ResourceId: &provider.ResourceId{OpaqueId: "abcd"},
			Owner:      user.GetId(),
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: grantee.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &provider.ResourcePermissions{
					GetPath:              true,
					InitiateFileDownload: true,
					ListFileVersions:     true,
					ListContainer:        true,
					Stat:                 true,
				},
			},
		}
		share2 = &collaboration.Share{
			Id:         &collaboration.ShareId{OpaqueId: "2"},
			ResourceId: &provider.ResourceId{OpaqueId: "efgh"},
			Owner:      user.GetId(),
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: grantee.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &provider.ResourcePermissions{
					GetPath:              true,
					InitiateFileDownload: true,
					ListFileVersions:     true,
					ListContainer:        true,
					Stat:                 true,
				},
			},
		}
		groupShare = &collaboration.Share{
			Id:         &collaboration.ShareId{OpaqueId: "3"},
			ResourceId: &provider.ResourceId{OpaqueId: "ijkl"},
			Owner:      user.GetId(),
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id: &provider.Grantee_GroupId{GroupId: &groupv1beta1.GroupId{
					Idp:      "localhost:1111",
					OpaqueId: "users",
				}},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: &provider.ResourcePermissions{
					GetPath:              true,
					InitiateFileDownload: true,
					ListFileVersions:     true,
					ListContainer:        true,
					Stat:                 true,
				},
			},
		}
		ctx = ctxpkg.ContextSetUser(context.Background(), user)
		granteeCtx = ctxpkg.ContextSetUser(context.Background(), grantee)
	})

	Describe("New", func() {
		JustBeforeEach(func() {
			m, err := cs3.New(nil, storage, indexer)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).ToNot(BeNil())
		})

		It("does not initialize the storage yet", func() {
			storage.AssertNotCalled(GinkgoT(), "Init", mock.Anything, mock.Anything)
		})
	})

	Describe("Load", func() {
		It("loads shares including state and mountpoint information", func() {
			m, err := cs3.New(nil, storage, indexer)
			Expect(err).ToNot(HaveOccurred())

			sharesChan := make(chan *collaboration.Share)
			receivedChan := make(chan sharespkg.ReceivedShareWithUser)

			wg := sync.WaitGroup{}
			wg.Add(2)
			go func() {
				err := m.Load(ctx, sharesChan, receivedChan)
				Expect(err).ToNot(HaveOccurred())
				wg.Done()
			}()
			go func() {
				sharesChan <- share
				close(sharesChan)
				close(receivedChan)
				wg.Done()
			}()
			wg.Wait()
			Eventually(sharesChan).Should(BeClosed())
			Eventually(receivedChan).Should(BeClosed())

			expectedPath := path.Join("shares", share.Id.OpaqueId)
			storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, expectedPath, mock.Anything)
		})
	})

	Context("with a manager instance and a share", func() {
		var (
			m              *cs3.Manager
			sharedResource = &provider.ResourceInfo{
				Id: &provider.ResourceId{
					StorageId: "storageid",
					OpaqueId:  "opaqueid",
				},
			}
		)

		JustBeforeEach(func() {
			var err error
			m, err = cs3.New(nil, storage, indexer)
			Expect(err).ToNot(HaveOccurred())
			data, err := json.Marshal(share)
			Expect(err).ToNot(HaveOccurred())
			storage.On("SimpleDownload", mock.Anything, path.Join("shares", share.Id.OpaqueId)).Return(data, nil)
			data, err = json.Marshal(share2)
			Expect(err).ToNot(HaveOccurred())
			storage.On("SimpleDownload", mock.Anything, path.Join("shares", share2.Id.OpaqueId)).Return(data, nil)
			data, err = json.Marshal(groupShare)
			Expect(err).ToNot(HaveOccurred())
			storage.On("SimpleDownload", mock.Anything, path.Join("shares", groupShare.Id.OpaqueId)).Return(data, nil)
			data, err = json.Marshal(&cs3.ReceivedShareMetadata{
				State: collaboration.ShareState_SHARE_STATE_PENDING,
				MountPoint: &provider.Reference{
					ResourceId: &provider.ResourceId{
						StorageId: "storageid",
						OpaqueId:  "opaqueid",
					},
					Path: "path",
				},
			})
			Expect(err).ToNot(HaveOccurred())
			storage.On("SimpleDownload", mock.Anything, path.Join("metadata", share2.Id.OpaqueId, granteeFn)).
				Return(data, nil)
			storage.On("SimpleDownload", mock.Anything, mock.Anything).Return(nil, errtypes.NotFound(""))
		})

		Describe("Share", func() {
			var (
				share *collaboration.Share
			)

			JustBeforeEach(func() {
				var err error
				share, err = m.Share(ctx, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns a share holding the share information", func() {
				Expect(share).ToNot(BeNil())
				Expect(share.ResourceId).To(Equal(sharedResource.Id))
				Expect(share.Creator).To(Equal(user.Id))
				Expect(share.Grantee).To(Equal(grant.Grantee))
				Expect(share.Permissions).To(Equal(grant.Permissions))
			})

			It("stores the share in the storage using the id as the filename", func() {
				expectedPath := path.Join("shares", share.Id.OpaqueId)
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, expectedPath, mock.Anything)
			})

			It("indexes the share", func() {
				indexer.AssertCalled(GinkgoT(), "Add", mock.AnythingOfType("*collaborationv1beta1.Share"))
			})
		})

		Describe("Unshare", func() {
			It("deletes the share", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(nil)
				err := m.Unshare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Id}})
				Expect(err).ToNot(HaveOccurred())
				expectedPath := path.Join("shares", share.Id.OpaqueId)
				storage.AssertCalled(GinkgoT(), "Delete", mock.Anything, expectedPath)
			})

			It("removes the share from the index", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(nil)
				err := m.Unshare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Id}})
				Expect(err).ToNot(HaveOccurred())
				indexer.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("*collaborationv1beta1.Share"))
			})

			It("still tries to delete the share from the index when the share couldn't be found", func() {
				storage.On("Delete", mock.Anything, mock.Anything).Return(errtypes.NotFound(""))
				err := m.Unshare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Id}})
				Expect(err).ToNot(HaveOccurred())
				indexer.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("*collaborationv1beta1.Share"))
			})
		})

		Describe("UpdateShare", func() {
			It("updates the share", func() {
				Expect(share.Permissions.Permissions.AddGrant).To(BeFalse())
				s, err := m.UpdateShare(ctx,
					&collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Id}},
					&collaboration.SharePermissions{Permissions: &provider.ResourcePermissions{AddGrant: true}}, nil, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(s).ToNot(BeNil())
				Expect(s.Permissions.Permissions.AddGrant).To(BeTrue())
			})
		})
		Describe("GetShare", func() {
			Context("when the share is a user share ", func() {
				Context("when requesting the share by id", func() {
					It("returns NotFound", func() {
						returnedShare, err := m.GetShare(ctx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: "1000"}},
						})
						Expect(err).To(HaveOccurred())
						Expect(returnedShare).To(BeNil())
					})

					It("returns the share", func() {
						returnedShare, err := m.GetShare(ctx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: "1"}},
						})
						Expect(err).ToNot(HaveOccurred())
						Expect(returnedShare).ToNot(BeNil())
						Expect(returnedShare.Id.OpaqueId).To(Equal(share.Id.OpaqueId))
						Expect(returnedShare.Owner).To(Equal(share.Owner))
						Expect(returnedShare.Grantee).To(Equal(share.Grantee))
						Expect(returnedShare.Permissions).To(Equal(share.Permissions))
					})
				})

				Context("when requesting the share by key", func() {
					It("returns NotFound", func() {
						indexer.On("FindBy", mock.Anything,
							indexerpkg.NewField("OwnerId", url.QueryEscape(share.Owner.Idp+":"+share.Owner.OpaqueId)),
						).
							Return([]string{share.Id.OpaqueId, share2.Id.OpaqueId}, nil)
						indexer.On("FindBy", mock.Anything,
							indexerpkg.NewField("GranteeId", url.QueryEscape("user:"+grantee.Id.Idp+":"+grantee.Id.OpaqueId)),
						).
							Return([]string{}, nil)
						returnedShare, err := m.GetShare(ctx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Key{
								Key: &collaboration.ShareKey{
									Owner:      share2.Owner,
									ResourceId: share2.ResourceId,
									Grantee:    share2.Grantee,
								},
							},
						})
						Expect(err).To(HaveOccurred())
						Expect(returnedShare).To(BeNil())
					})
					It("gets the share", func() {
						indexer.On("FindBy", mock.Anything,
							indexerpkg.NewField("OwnerId", url.QueryEscape(share.Owner.Idp+":"+share.Owner.OpaqueId)),
						).
							Return([]string{share.Id.OpaqueId, share2.Id.OpaqueId}, nil)
						indexer.On("FindBy", mock.Anything,
							indexerpkg.NewField("GranteeId", url.QueryEscape("user:"+grantee.Id.Idp+":"+grantee.Id.OpaqueId)),
						).
							Return([]string{share2.Id.OpaqueId}, nil)
						returnedShare, err := m.GetShare(ctx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Key{
								Key: &collaboration.ShareKey{
									Owner:      share2.Owner,
									ResourceId: share2.ResourceId,
									Grantee:    share2.Grantee,
								},
							},
						})
						Expect(err).ToNot(HaveOccurred())
						Expect(returnedShare).ToNot(BeNil())
						Expect(returnedShare.Id.OpaqueId).To(Equal(share2.Id.OpaqueId))
						Expect(returnedShare.Owner).To(Equal(share2.Owner))
						Expect(returnedShare.Grantee).To(Equal(share2.Grantee))
						Expect(returnedShare.Permissions).To(Equal(share2.Permissions))
					})
				})
			})

			Context("when the share is a group share ", func() {
				BeforeEach(func() {
					share.Grantee = &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
						Id: &provider.Grantee_GroupId{
							GroupId: &groupv1beta1.GroupId{OpaqueId: "1000"},
						},
					}
				})

				It("returns a group share", func() {
					returnedShare, err := m.GetShare(ctx, &collaboration.ShareReference{
						Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: "1"}},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(returnedShare).ToNot(BeNil())
					Expect(returnedShare.Id.OpaqueId).To(Equal(share.Id.OpaqueId))
					Expect(returnedShare.Owner).To(Equal(share.Owner))
					Expect(returnedShare.Grantee).To(Equal(share.Grantee))
					Expect(returnedShare.Permissions).To(Equal(share.Permissions))
				})
			})
		})

		Describe("ListShares", func() {
			JustBeforeEach(func() {
				indexer.On("FindBy", mock.Anything,
					mock.MatchedBy(func(input indexerpkg.Field) bool {
						return input.Name == "OwnerId"
					}),
					mock.MatchedBy(func(input indexerpkg.Field) bool {
						return input.Name == "CreatorId"
					}),
				).Return([]string{share.Id.OpaqueId, share2.Id.OpaqueId}, nil)
				indexer.On("FindBy", mock.Anything,
					mock.MatchedBy(func(input indexerpkg.Field) bool {
						return input.Name == "ResourceId" && input.Value == "!abcd"
					}),
				).Return([]string{share.Id.OpaqueId}, nil)
			})
			It("uses the index to get the owned shares", func() {
				shares, err := m.ListShares(ctx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(2))
				Expect(shares[0].Id.OpaqueId).To(Equal("1"))
				Expect(shares[1].Id.OpaqueId).To(Equal("2"))
			})

			It("applies resource id filters", func() {
				shares, err := m.ListShares(ctx, []*collaboration.Filter{
					{
						Type: collaboration.Filter_TYPE_RESOURCE_ID,
						Term: &collaboration.Filter_ResourceId{
							ResourceId: share.ResourceId,
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
				Expect(shares[0].Id.OpaqueId).To(Equal("1"))
			})
		})

		Describe("ListReceivedShares", func() {
			Context("with a received user share", func() {
				BeforeEach(func() {
					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId" && input.Value == granteeFn
						}),
					).
						Return([]string{share2.Id.OpaqueId}, nil)
					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId"
						}),
					).
						Return([]string{}, nil)
				})

				It("list the user shares", func() {
					rshares, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{}, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(rshares).ToNot(BeNil())
					Expect(len(rshares)).To(Equal(1))

					rshare := rshares[0]
					Expect(rshare.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
					Expect(rshare.MountPoint.ResourceId.StorageId).To(Equal("storageid"))
					Expect(rshare.MountPoint.ResourceId.OpaqueId).To(Equal("opaqueid"))
					Expect(rshare.MountPoint.Path).To(Equal("path"))
				})
			})

			Context("with a received group share", func() {
				BeforeEach(func() {
					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId" && input.Value == groupFn
						}),
					).
						Return([]string{share2.Id.OpaqueId}, nil)
					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId"
						}),
					).
						Return([]string{}, nil)
				})

				It("list the group share", func() {
					rshares, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{}, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(rshares).ToNot(BeNil())
					Expect(len(rshares)).To(Equal(1))

					rshare := rshares[0]
					Expect(rshare.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
					Expect(rshare.MountPoint.ResourceId.StorageId).To(Equal("storageid"))
					Expect(rshare.MountPoint.ResourceId.OpaqueId).To(Equal("opaqueid"))
					Expect(rshare.MountPoint.Path).To(Equal("path"))
				})
			})

			Context("with a received user and group share", func() {
				BeforeEach(func() {
					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId" && input.Value == granteeFn
						}),
					).
						Return([]string{share.Id.OpaqueId}, nil)

					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId" && input.Value == groupFn
						}),
					).
						Return([]string{groupShare.Id.OpaqueId}, nil)

					indexer.On("FindBy", mock.Anything,
						mock.MatchedBy(func(input indexerpkg.Field) bool {
							return input.Name == "GranteeId"
						}),
					).
						Return([]string{}, nil)
				})

				It("list the user and shares", func() {
					rshares, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{}, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(rshares).ToNot(BeNil())
					Expect(len(rshares)).To(Equal(2))
				})

				It("list only the user when user filter is given ", func() {
					rshares, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{
						{
							Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
							Term: &collaboration.Filter_GranteeType{
								GranteeType: provider.GranteeType_GRANTEE_TYPE_USER,
							},
						},
					}, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(rshares).ToNot(BeNil())
					Expect(len(rshares)).To(Equal(1))
					Expect(rshares[0].Share.Grantee.Type).To(Equal(provider.GranteeType_GRANTEE_TYPE_USER))
				})

				It("list only the group when group filter is given ", func() {
					rshares, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{
						{
							Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
							Term: &collaboration.Filter_GranteeType{
								GranteeType: provider.GranteeType_GRANTEE_TYPE_GROUP,
							},
						},
					}, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(rshares).ToNot(BeNil())
					Expect(len(rshares)).To(Equal(1))
					Expect(rshares[0].Share.Grantee.Type).To(Equal(provider.GranteeType_GRANTEE_TYPE_GROUP))
				})
			})
		})

		Describe("GetReceivedShare", func() {
			Context("when the share is a user share ", func() {
				Context("when requesting the share by id", func() {
					It("returns NotFound", func() {
						rshare, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: "1000"}},
						})
						Expect(err).To(HaveOccurred())
						Expect(rshare).To(BeNil())
					})

					It("returns the share", func() {
						rshare, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{
							Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: share2.Id.OpaqueId}},
						})
						Expect(err).ToNot(HaveOccurred())
						Expect(rshare).ToNot(BeNil())
						Expect(rshare.Share.Id.OpaqueId).To(Equal(share2.Id.OpaqueId))
						Expect(rshare.Share.Owner).To(Equal(share2.Owner))
						Expect(rshare.Share.Grantee).To(Equal(share2.Grantee))
						Expect(rshare.Share.Permissions).To(Equal(share2.Permissions))
						Expect(rshare.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
						Expect(rshare.MountPoint.ResourceId.StorageId).To(Equal("storageid"))
						Expect(rshare.MountPoint.ResourceId.OpaqueId).To(Equal("opaqueid"))
						Expect(rshare.MountPoint.Path).To(Equal("path"))
					})
				})
			})
		})

		Describe("UpdateReceivedShare", func() {
			It("updates the share", func() {
				rs, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share2.Id}})
				Expect(err).ToNot(HaveOccurred())

				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
				rs.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
				rs.MountPoint.Path = "newPath/"

				rrs, err := m.UpdateReceivedShare(granteeCtx,
					rs, &fieldmaskpb.FieldMask{Paths: []string{"state", "mount_point"}}, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(rrs).ToNot(BeNil())
				Expect(rrs.Share.ResourceId).ToNot(BeNil())
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, "metadata/2/"+granteeFn, mock.MatchedBy(func(data []byte) bool {
					meta := cs3.ReceivedShareMetadata{}
					err := json.Unmarshal(data, &meta)
					Expect(err).ToNot(HaveOccurred())
					return meta.MountPoint != nil && meta.State == collaboration.ShareState_SHARE_STATE_ACCEPTED && meta.MountPoint.Path == "newPath/"
				}))
			})

			It("does not update fields that aren't part of the fieldmask", func() {
				rs, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share2.Id}})
				Expect(err).ToNot(HaveOccurred())

				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
				rs.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
				rs.MountPoint.Path = "newPath/"

				rrs, err := m.UpdateReceivedShare(granteeCtx,
					rs, &fieldmaskpb.FieldMask{Paths: []string{"mount_point"}}, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(rrs).ToNot(BeNil())
				Expect(rrs.Share.ResourceId).ToNot(BeNil())
				storage.AssertCalled(GinkgoT(), "SimpleUpload", mock.Anything, "metadata/2/"+granteeFn, mock.MatchedBy(func(data []byte) bool {
					meta := cs3.ReceivedShareMetadata{}
					err := json.Unmarshal(data, &meta)
					Expect(err).ToNot(HaveOccurred())
					return meta.MountPoint != nil && meta.State == collaboration.ShareState_SHARE_STATE_PENDING && meta.MountPoint.Path == "newPath/"
				}))
			})
		})
	})
})
