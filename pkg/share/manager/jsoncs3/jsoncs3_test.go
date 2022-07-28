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

package jsoncs3_test

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3"
	storagemocks "github.com/cs3org/reva/v2/pkg/storage/utils/metadata/mocks"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Json", func() {
	var (
		user1 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "admin",
			},
		}
		user2 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "einstein",
			},
		}

		sharedResource = &providerv1beta1.ResourceInfo{
			Id: &providerv1beta1.ResourceId{
				StorageId: "storageid",
				SpaceId:   "spaceid",
				OpaqueId:  "opaqueid",
			},
		}

		grantee = &userpb.User{
			Id:     user2.Id,
			Groups: []string{"users"},
		}
		readPermissions = &provider.ResourcePermissions{
			GetPath:              true,
			InitiateFileDownload: true,
			ListFileVersions:     true,
			ListContainer:        true,
			Stat:                 true,
		}
		writePermissions = &provider.ResourcePermissions{
			GetPath:              true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
			ListFileVersions:     true,
			ListContainer:        true,
			Stat:                 true,
		}
		grant = &collaboration.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: grantee.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: readPermissions,
			},
		}

		storage    *storagemocks.Storage
		m          share.Manager
		ctx        context.Context
		granteeCtx context.Context

		// helper functions
		shareBykey = func(key *collaboration.ShareKey) *collaboration.Share {
			s, err := m.GetShare(ctx, &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Key{
					Key: key,
				},
			})
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, s).ToNot(BeNil())
			return s
		}
	)

	BeforeEach(func() {
		storage = &storagemocks.Storage{}
		storage.On("Init", mock.Anything, mock.Anything).Return(nil)
		storage.On("MakeDirIfNotExist", mock.Anything, mock.Anything).Return(nil)
		storage.On("SimpleUpload", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		var err error
		m, err = jsoncs3.New(storage)
		Expect(err).ToNot(HaveOccurred())

		ctx = ctxpkg.ContextSetUser(context.Background(), user1)
		granteeCtx = ctxpkg.ContextSetUser(context.Background(), user2)
	})

	Describe("Share", func() {
		It("fails if the share already exists", func() {
			_, err := m.Share(ctx, sharedResource, grant)
			Expect(err).ToNot(HaveOccurred())
			_, err = m.Share(ctx, sharedResource, grant)
			Expect(err).To(HaveOccurred())
		})

		It("creates a share", func() {
			share, err := m.Share(ctx, sharedResource, grant)
			Expect(err).ToNot(HaveOccurred())

			Expect(share).ToNot(BeNil())
			Expect(share.ResourceId).To(Equal(sharedResource.Id))
		})
	})

	Context("with an existing share", func() {
		var (
			share *collaboration.Share
		)

		BeforeEach(func() {
			var err error
			share, err = m.Share(ctx, sharedResource, grant)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("GetShare", func() {
			It("retrieves an existing share by id", func() {
				s, err := m.GetShare(ctx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: &collaboration.ShareId{
							OpaqueId: share.Id.OpaqueId,
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(s).ToNot(BeNil())
				Expect(share.ResourceId).To(Equal(sharedResource.Id))
			})

			It("retrieves an existing share by key", func() {
				s := shareBykey(&collaboration.ShareKey{
					ResourceId: sharedResource.Id,
					Grantee:    grant.Grantee,
				})
				Expect(s.ResourceId).To(Equal(sharedResource.Id))
				Expect(s.Id.OpaqueId).To(Equal(share.Id.OpaqueId))
			})
		})

		Describe("UnShare", func() {
			It("removes an existing share", func() {
				err := m.Unshare(ctx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: &collaboration.ShareId{
							OpaqueId: share.Id.OpaqueId,
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())

				s, err := m.GetShare(ctx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Key{
						Key: &collaboration.ShareKey{
							ResourceId: sharedResource.Id,
							Grantee:    grant.Grantee,
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(s).To(BeNil())
			})
		})

		Describe("UpdateShare", func() {
			It("updates an existing share", func() {
				s := shareBykey(&collaboration.ShareKey{
					ResourceId: sharedResource.Id,
					Grantee:    grant.Grantee,
				})
				Expect(s.GetPermissions().GetPermissions()).To(Equal(readPermissions))

				// enhance privileges
				us, err := m.UpdateShare(ctx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: &collaboration.ShareId{
							OpaqueId: share.Id.OpaqueId,
						},
					},
				}, &collaboration.SharePermissions{
					Permissions: writePermissions,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(us).ToNot(BeNil())
				Expect(us.GetPermissions().GetPermissions()).To(Equal(writePermissions))

				s = shareBykey(&collaboration.ShareKey{
					ResourceId: sharedResource.Id,
					Grantee:    grant.Grantee,
				})
				Expect(s.GetPermissions().GetPermissions()).To(Equal(writePermissions))

				// reduce privileges
				us, err = m.UpdateShare(ctx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: &collaboration.ShareId{
							OpaqueId: share.Id.OpaqueId,
						},
					},
				}, &collaboration.SharePermissions{
					Permissions: readPermissions,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(us).ToNot(BeNil())
				Expect(us.GetPermissions().GetPermissions()).To(Equal(readPermissions))

				s = shareBykey(&collaboration.ShareKey{
					ResourceId: sharedResource.Id,
					Grantee:    grant.Grantee,
				})
				Expect(s.GetPermissions().GetPermissions()).To(Equal(readPermissions))
			})
		})

		Describe("ListShares", func() {
			It("lists an existing share", func() {
				shares, err := m.ListShares(ctx, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(HaveLen(1))

				Expect(shares[0].Id).To(Equal(share.Id))
			})
		})

		Describe("ListReceivedShares", func() {
			PIt("filters by resource id")
			PIt("filters by owner")
			PIt("filters by creator")
			PIt("filters by grantee type")

			It("lists the received shares", func() {
				received, err := m.ListReceivedShares(granteeCtx, []*collaboration.Filter{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(received)).To(Equal(1))
				Expect(received[0].Share.ResourceId).To(Equal(sharedResource.Id))
				Expect(received[0].State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
			})
		})

		Describe("GetReceivedShare", func() {
			It("gets the state", func() {
				rs, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: share.Id,
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))
			})
		})

		Describe("UpdateReceivedShare", func() {
			It("updates the state", func() {
				rs, err := m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: share.Id,
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_PENDING))

				rs.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
				rs, err = m.UpdateReceivedShare(granteeCtx, rs, &fieldmaskpb.FieldMask{Paths: []string{"state"}})
				Expect(err).ToNot(HaveOccurred())
				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))

				rs, err = m.GetReceivedShare(granteeCtx, &collaboration.ShareReference{
					Spec: &collaboration.ShareReference_Id{
						Id: share.Id,
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(rs.State).To(Equal(collaboration.ShareState_SHARE_STATE_ACCEPTED))
			})
		})
	})
})
