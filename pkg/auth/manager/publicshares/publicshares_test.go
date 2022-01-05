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

package publicshares

import (
	"context"

	userprovider "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/manager/publicshares/mocks"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("PublicShareAuthentication", func() {
	var (
		gwc *mocks.GatewayClient
		mgr manager
		ctx context.Context
	)

	BeforeEach(func() {
		gwc = &mocks.GatewayClient{}
		mgr = manager{
			gwc: gwc,
		}
		ctx = context.Background()

		// common mocks
	})

	Describe("when authenticating ", func() {

		shareCreator := &userprovider.UserId{
			Idp:      "https://idp.owncloud.test",
			OpaqueId: "some-user-id",
			Type:     userprovider.UserType_USER_TYPE_PRIMARY,
		}
		Context("on an public link without required password", func() {

			It("succeeds", func() {
				// individual mocks
				gwc.Mock.On(
					"GetPublicShareByToken",
					mock.Anything, // ctx
					&linkv1beta1.GetPublicShareByTokenRequest{
						Opaque:         nil,
						Token:          "public-token-123",
						Authentication: nil,
						Sign:           true,
					},
				).Return(
					&linkv1beta1.GetPublicShareByTokenResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						Share: &linkv1beta1.PublicShare{
							Id: &linkv1beta1.PublicShareId{
								OpaqueId: "some-public-share-id",
							},
							Token: "public-token-123",
							ResourceId: &providerv1beta1.ResourceId{
								StorageId: "real-storage-id",
								OpaqueId:  "file-id",
							},
							Permissions: &linkv1beta1.PublicSharePermissions{
								Permissions: &providerv1beta1.ResourcePermissions{
									Stat: true,
								},
							},
							Creator: shareCreator,
						},
					},
					nil,
				).Once()

				gwc.Mock.On(
					"GetUser",
					mock.Anything, // ctx
					&userprovider.GetUserRequest{
						Opaque:                 nil,
						UserId:                 shareCreator,
						SkipFetchingUserGroups: false,
					},
				).Return(
					&userprovider.GetUserResponse{
						Status: &rpc.Status{Code: rpc.Code_CODE_OK},
						Opaque: &typesv1beta1.Opaque{},
						User: &userprovider.User{
							Id:       shareCreator,
							Username: "Testuser",
						},
					},
					nil,
				).Once()

				// the actual request
				_, token, err := mgr.Authenticate(ctx, "public-token-123", "")

				// check response
				Expect(err).To(BeNil())
				Expect(token).To(Not(BeEmpty()))
			})
		})
	})
})
