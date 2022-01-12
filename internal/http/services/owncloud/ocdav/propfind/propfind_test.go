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

package propfind_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	sprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/propfind/mocks"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Propfind", func() {
	var (
		handler *propfind.Handler
		client  *mocks.GatewayClient
		ctx     context.Context
	)

	JustBeforeEach(func() {
		ctx = context.Background()
		client = &mocks.GatewayClient{}
		handler = propfind.NewHandler("127.0.0.1:3000", func() (propfind.GatewayClient, error) {
			return client, nil
		})
	})

	Describe("NewHandler", func() {
		It("returns a handler", func() {
			Expect(handler).ToNot(BeNil())
		})
	})

	Describe("HandleSpacesPropfind", func() {
		It("handles invalid space ids", func() {
			client.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(&sprovider.ListStorageSpacesResponse{
				Status:        status.NewOK(ctx),
				StorageSpaces: []*sprovider.StorageSpace{},
			}, nil)

			rr := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/", strings.NewReader(""))
			Expect(err).ToNot(HaveOccurred())

			handler.HandleSpacesPropfind(rr, req, "foo")
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})
	})
})
