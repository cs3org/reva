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
	"strconv"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ruser "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/share"
	sqlmanager "github.com/cs3org/reva/v2/pkg/share/manager/owncloudsql"
	mocks "github.com/cs3org/reva/v2/pkg/share/manager/owncloudsql/mocks"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SQL manager", func() {
	var (
		mgr        share.Manager
		ctx        context.Context
		testDbFile *os.File
		sqldb      *sql.DB

		loginAs = func(user *userpb.User) {
			ctx = ruser.ContextSetUser(context.Background(), user)
		}
		admin = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "admin",
		}
		otherUser = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
			Groups:   []string{"users"},
		}
		yetAnotherUser = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid2",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "marie",
			Groups:   []string{"users"},
		}

		shareRef = &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{
			Id: &collaboration.ShareId{
				OpaqueId: "1",
			},
		}}

		insertShare = func(shareType int, owner string, grantee string, parent int, source int, fileTarget string, permissions int, accepted int) (int, error) {
			var parentVal interface{}
			if parent >= 0 {
				parentVal = parent
			}
			stmtString := "INSERT INTO oc_share (share_type,uid_owner,uid_initiator,item_type,item_source,file_source,parent,permissions,stime,share_with,file_target,accepted) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)"
			stmtValues := []interface{}{shareType, owner, owner, "folder", source, source, parentVal, permissions, 1631779730, grantee, fileTarget, accepted}

			stmt, err := sqldb.Prepare(stmtString)
			if err != nil {
				return -1, err
			}
			result, err := stmt.ExecContext(ctx, stmtValues...)
			if err != nil {
				return -1, err
			}
			id, err := result.LastInsertId()
			return int(id), err
		}
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
		mgr, err = sqlmanager.New("sqlite3", sqldb, "abcde", userConverter)
		Expect(err).ToNot(HaveOccurred())

		loginAs(admin)
	})

	It("creates manager instances", func() {
		Expect(mgr).ToNot(BeNil())
	})

	Describe("GetShare", func() {
		It("returns the share", func() {
			share, err := mgr.GetShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
		})

		It("returns an error if the share does not exis", func() {
			share, err := mgr.GetShare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: "2",
				},
			}})
			Expect(err).To(HaveOccurred())
			Expect(share).To(BeNil())
		})
	})

	Describe("Share", func() {
		It("creates a share", func() {
			grant := &collaboration.ShareGrant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{UserId: &user.UserId{
						OpaqueId: "someone",
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
			info := &provider.ResourceInfo{
				Id: &provider.ResourceId{
					SpaceId:  "/",
					OpaqueId: "something",
				},
			}
			share, err := mgr.Share(ctx, info, grant)

			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.Id.OpaqueId).To(Equal("2"))
		})
	})

	Describe("ListShares", func() {
		It("lists shares", func() {
			shares, err := mgr.ListShares(ctx, []*collaboration.Filter{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(1))
			share := shares[0]
			Expect(share.ResourceId.SpaceId).To(Equal("1"))

			shares, err = mgr.ListShares(ctx, []*collaboration.Filter{
				{
					Type: collaboration.Filter_TYPE_RESOURCE_ID,
					Term: &collaboration.Filter_ResourceId{
						ResourceId: &provider.ResourceId{
							SpaceId:  "/",
							OpaqueId: "somethingElse",
						},
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(0))
		})
	})

	Describe("ListReceivedShares", func() {
		Context("with a pending group share (non-autoaccept) and an accepted child share", func() {
			It("only returns one share (of type group share)", func() {
				loginAs(otherUser)
				parentID, err := insertShare(
					1,              // group share
					"admin",        // owner/initiator
					"users",        // grantee
					-1,             // parent
					20,             // source
					"/groupshared", // file_target
					31,             // permissions,
					0,              // accepted
				)
				Expect(err).ToNot(HaveOccurred())
				_, err = insertShare(
					2,                // group child share
					"admin",          // owner/initiator
					"einstein",       // grantee
					parentID,         // parent
					20,               // source
					"/mygroupshared", // file_target
					31,               // permissions,
					0,                // accepted
				)
				Expect(err).ToNot(HaveOccurred())

				shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(2))
				groupShare := shares[1]
				Expect(groupShare.MountPoint.Path).To(Equal("mygroupshared"))
				Expect(groupShare.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))
				Expect(groupShare.Share.Id.OpaqueId).To(Equal(strconv.Itoa(parentID)))
				Expect(groupShare.Share.Grantee.Type).To(Equal(provider.GranteeType_GRANTEE_TYPE_GROUP))
				Expect(groupShare.Share.Grantee.GetGroupId().OpaqueId).To(Equal("users"))
			})
		})

		Context("with an accepted group share", func() {
			It("lists the group share too", func() {
				loginAs(otherUser)
				_, err := insertShare(
					1,         // group share
					"admin",   // owner/initiator
					"users",   // grantee
					-1,        // parent
					20,        // source
					"/shared", // file_target
					31,        // permissions,
					0,         // accepted
				)
				Expect(err).ToNot(HaveOccurred())

				shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(2))
				groupShare := shares[1]
				Expect(groupShare.MountPoint.Path).To(Equal("shared"))
				Expect(groupShare.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))
				Expect(groupShare.Share.Grantee.Type).To(Equal(provider.GranteeType_GRANTEE_TYPE_GROUP))
			})

			It("lists the child share information if the user changed the mountpoint", func() {
				loginAs(otherUser)
				parentID, err := insertShare(
					1,              // group share
					"admin",        // owner/initiator
					"users",        // grantee
					-1,             // parent
					20,             // source
					"/groupshared", // file_target
					31,             // permissions,
					1,              // accepted
				)
				Expect(err).ToNot(HaveOccurred())
				_, err = insertShare(
					2,                // group child share
					"admin",          // owner/initiator
					"einstein",       // grantee
					parentID,         // parent
					20,               // source
					"/mygroupshared", // file_target
					31,               // permissions,
					0,                // accepted
				)
				Expect(err).ToNot(HaveOccurred())

				shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(2))
				groupShare := shares[1]
				Expect(groupShare.MountPoint.Path).To(Equal("mygroupshared"))
				Expect(groupShare.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))
				Expect(groupShare.Share.Id.OpaqueId).To(Equal(strconv.Itoa(parentID)))
				Expect(groupShare.Share.Grantee.Type).To(Equal(provider.GranteeType_GRANTEE_TYPE_GROUP))
				Expect(groupShare.Share.Grantee.GetGroupId().OpaqueId).To(Equal("users"))
			})

			It("does not lists group shares named like the user", func() {
				loginAs(otherUser)
				_, err := insertShare(
					1,          // group share
					"admin",    // owner/initiator
					"einstein", // grantee
					-1,         // parent
					20,         // source
					"/shared",  // file_target
					31,         // permissions,
					0,          // accepted
				)
				Expect(err).ToNot(HaveOccurred())

				shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
			})
		})

		It("lists received shares", func() {
			loginAs(otherUser)
			shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(1))
		})

		It("works with filters", func() {
			loginAs(otherUser)
			shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{{Type: collaboration.Filter_TYPE_EXCLUDE_DENIALS}})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(1))
		})
	})

	Describe("GetReceivedShare", func() {
		It("returns the received share", func() {
			loginAs(otherUser)
			share, err := mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
		})
	})

	Describe("UpdateReceivedShare", func() {
		It("returns an error when no valid field is set in the mask", func() {
			loginAs(otherUser)

			share, err := mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

			share.State = collaboration.ShareState_SHARE_STATE_REJECTED
			_, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"foo"}})
			Expect(err).To(HaveOccurred())
		})

		It("updates the state when the state is set in the mask", func() {
			loginAs(otherUser)

			share, err := mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

			share.State = collaboration.ShareState_SHARE_STATE_REJECTED

			share, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"mount_point"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

			share.State = collaboration.ShareState_SHARE_STATE_REJECTED
			share, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"state"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_REJECTED))

			share, err = mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_REJECTED))
		})

		It("updates the mount_point when the mount_point is set in the mask", func() {
			loginAs(otherUser)

			share, err := mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

			share.MountPoint = &provider.Reference{Path: "foo"}

			share, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"state"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(share.MountPoint.Path).To(Equal("shared"))

			share.MountPoint = &provider.Reference{Path: "foo"}
			share, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"mount_point"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(share.MountPoint.Path).To(Equal("foo"))

			share, err = mgr.GetReceivedShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share).ToNot(BeNil())
			Expect(share.MountPoint.Path).To(Equal("foo"))
		})

		Context("with a group share", func() {
			It("updates the child share with the custom information", func() {
				loginAs(otherUser)
				parentID, err := insertShare(
					1,              // group share
					"admin",        // owner/initiator
					"users",        // grantee
					-1,             // parent
					20,             // source
					"/groupshared", // file_target
					31,             // permissions,
					1,              // accepted
				)
				Expect(err).ToNot(HaveOccurred())
				_, err = insertShare(
					2,                // group child share
					"admin",          // owner/initiator
					"einstein",       // grantee
					parentID,         // parent
					20,               // source
					"/mygroupshared", // file_target
					31,               // permissions,
					0,                // accepted
				)
				Expect(err).ToNot(HaveOccurred())
				parentRef := &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: strconv.Itoa(parentID),
					},
				}}

				share, err := mgr.GetReceivedShare(ctx, parentRef)
				Expect(err).ToNot(HaveOccurred())
				Expect(share).ToNot(BeNil())
				Expect(share.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

				share.MountPoint = &provider.Reference{Path: "foo"}

				By("overriding the child share information for the current user")
				share, err = mgr.UpdateReceivedShare(ctx, share, &fieldmaskpb.FieldMask{Paths: []string{"mount_point"}})
				Expect(err).ToNot(HaveOccurred())
				Expect(share.MountPoint.Path).To(Equal("foo"))

				share, err = mgr.GetReceivedShare(ctx, parentRef)
				Expect(err).ToNot(HaveOccurred())
				Expect(share).ToNot(BeNil())
				Expect(share.MountPoint.Path).To(Equal("foo"))

				By("not overriding the parent share information")
				loginAs(yetAnotherUser)
				share, err = mgr.GetReceivedShare(ctx, parentRef)
				Expect(err).ToNot(HaveOccurred())
				Expect(share.MountPoint.Path).To(Equal("groupshared"))
			})
		})
	})

	Describe("Unshare", func() {
		It("deletes shares", func() {
			loginAs(otherUser)
			shares, err := mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(1))

			loginAs(admin)
			err = mgr.Unshare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: shares[0].Share.Id.OpaqueId,
				},
			}})
			Expect(err).ToNot(HaveOccurred())

			loginAs(otherUser)
			shares, err = mgr.ListReceivedShares(ctx, []*collaboration.Filter{})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(shares)).To(Equal(0))
		})
	})

	Describe("UpdateShare", func() {
		It("updates permissions", func() {
			share, err := mgr.GetShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share.Permissions.Permissions.Delete).To(BeTrue())

			share, err = mgr.UpdateShare(ctx, shareRef, &collaboration.SharePermissions{
				Permissions: &provider.ResourcePermissions{
					InitiateFileUpload: true,
					RestoreFileVersion: true,
					RestoreRecycleItem: true,
				}})
			Expect(err).ToNot(HaveOccurred())
			Expect(share.Permissions.Permissions.Delete).To(BeFalse())

			share, err = mgr.GetShare(ctx, shareRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(share.Permissions.Permissions.Delete).To(BeFalse())
		})
	})
})
