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

package sharestorageprovider_test

import (
	"context"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	sprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	provider "github.com/cs3org/reva/internal/grpc/services/sharestorageprovider"
	_ "github.com/cs3org/reva/pkg/share/manager/loader"
	sharemocks "github.com/cs3org/reva/pkg/share/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Sharestorageprovider", func() {
	var (
		config = map[string]interface{}{
			"mount_path":   "/shares",
			"gateway_addr": "127.0.0.1:1234",
			"driver":       "json",
			"drivers": map[string]map[string]interface{}{
				"json": map[string]interface{}{},
			},
		}
		ctx = context.Background()

		s  sprovider.ProviderAPIServer
		sm *sharemocks.Manager
	)

	BeforeEach(func() {
		sm = &sharemocks.Manager{}
	})

	JustBeforeEach(func() {
		p, err := provider.New("", nil, sm)
		Expect(err).ToNot(HaveOccurred())
		s = p.(sprovider.ProviderAPIServer)
		Expect(s).ToNot(BeNil())
	})

	Describe("NewDefault", func() {
		It("returns a new service instance", func() {
			s, err := provider.NewDefault(config, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(s).ToNot(BeNil())
		})
	})

	Describe("ListContainer", func() {
		var (
			req = &sprovider.ListContainerRequest{
				Ref: &sprovider.Reference{
					Spec: &sprovider.Reference_Path{Path: "/shares"},
				},
			}
		)

		It("lists shares", func() {
			sm.On("ListReceivedShares", mock.Anything).Return([]*collaboration.ReceivedShare{
				&collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
				},
			}, nil)
			res, err := s.ListContainer(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(1))

		})

		It("only considers accepted shares", func() {
			sm.On("ListReceivedShares", mock.Anything).Return([]*collaboration.ReceivedShare{
				&collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_INVALID,
				},
				&collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_PENDING,
				},
				&collaboration.ReceivedShare{
					State: collaboration.ShareState_SHARE_STATE_REJECTED,
				},
			}, nil)
			res, err := s.ListContainer(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(len(res.Infos)).To(Equal(0))
		})
	})
})
