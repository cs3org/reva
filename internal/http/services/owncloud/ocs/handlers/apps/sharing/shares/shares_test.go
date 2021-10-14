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

package shares_test

import (
	"context"
	"encoding/xml"
	"net/http/httptest"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares/mocks"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The ocs API", func() {
	var (
		h      *shares.Handler
		client *mocks.GatewayClient
	)

	BeforeEach(func() {
		h = &shares.Handler{}
		client = &mocks.GatewayClient{}

		c := &config.Config{}
		c.Init()
		h.Init(c, func() (shares.GatewayClient, error) {
			return client, nil
		})
	})

	Describe("ListShares", func() {
		BeforeEach(func() {
			resId := &provider.ResourceId{
				StorageId: "share1-storageid",
				OpaqueId:  "share1",
			}
			client.On("ListReceivedShares", mock.Anything, mock.Anything, mock.Anything).Return(&collaboration.ListReceivedSharesResponse{
				Status: status.NewOK(context.Background()),
				Shares: []*collaboration.ReceivedShare{
					{
						State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
						Share: &collaboration.Share{
							Id: &collaboration.ShareId{OpaqueId: "10"},
							Grantee: &provider.Grantee{
								Type: provider.GranteeType_GRANTEE_TYPE_USER,
							},
							ResourceId: resId,
							Permissions: &collaboration.SharePermissions{
								Permissions: &provider.ResourcePermissions{
									Stat:          true,
									ListContainer: true,
								},
							},
						},
						MountPoint: &provider.Reference{Path: "share1"},
					},
				},
			}, nil)

			client.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: status.NewOK(context.Background()),
				Info: &provider.ResourceInfo{
					Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Path: "/share1",
					Id:   resId,
					PermissionSet: &provider.ResourcePermissions{
						Stat: true,
					},
					Size: 10,
				},
			}, nil)

			client.On("ListContainer", mock.Anything, mock.Anything).Return(&provider.ListContainerResponse{
				Status: status.NewOK(context.Background()),
				Infos: []*provider.ResourceInfo{
					{
						Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
						Path: "/share1",
						Id:   resId,
						Size: 1,
					},
				},
			}, nil)
		})

		It("lists accepted shares", func() {
			type share struct {
				Id string `xml:"id"`
			}
			type data struct {
				Shares []share `xml:"element"`
			}
			type response struct {
				Data data `xml:"data"`
			}

			req := httptest.NewRequest("GET", "/apps/files_sharing/api/v1/shares?shared_with_me=1", nil)
			w := httptest.NewRecorder()
			h.ListShares(w, req)
			Expect(w.Result().StatusCode).To(Equal(200))

			res := &response{}
			err := xml.Unmarshal(w.Body.Bytes(), res)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res.Data.Shares)).To(Equal(1))
			s := res.Data.Shares[0]
			Expect(s.Id).To(Equal("10"))
		})
	})
})
