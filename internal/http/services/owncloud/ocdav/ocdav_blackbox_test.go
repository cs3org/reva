// Copyright 2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
package ocdav_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	cs3gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	cs3user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rhttp/global"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/trace"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TODO for now we have to test all of ocdav. when this testsuite is complete we can move
// the handlers to dedicated packages to reduce the amount of complexity to get a test environment up
var _ = Describe("ocdav", func() {
	var (
		handler global.Service
		client  *mocks.GatewayAPIClient
		ctx     context.Context

		userspace *cs3storageprovider.StorageSpace
		user      *cs3user.User
	)

	BeforeEach(func() {
		user = &cs3user.User{Id: &cs3user.UserId{OpaqueId: "username"}, Username: "username"}

		ctx = ctxpkg.ContextSetUser(context.Background(), user)
		client = &mocks.GatewayAPIClient{}

		var err error
		handler, err = ocdav.NewWith(&ocdav.Config{
			FilesNamespace:  "/users/{{.Username}}",
			WebdavNamespace: "/users/{{.Username}}",
		}, nil, ocdav.NewCS3LS(client), nil, trace.NewNoopTracerProvider(), client)
		Expect(err).ToNot(HaveOccurred())

		userspace = &cs3storageprovider.StorageSpace{
			Opaque: &typesv1beta1.Opaque{
				Map: map[string]*typesv1beta1.OpaqueEntry{
					"path": {
						Decoder: "plain",
						Value:   []byte("/users/username/"),
					},
				},
			},
			Id:   &cs3storageprovider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(cs3storageprovider.ResourceId{StorageId: "provider-1", SpaceId: "foospace", OpaqueId: "root"})},
			Root: &cs3storageprovider.ResourceId{StorageId: "provider-1", SpaceId: "userspace", OpaqueId: "root"},
			Name: "username",
		}

		client.On("GetUser", mock.Anything, mock.Anything).Return(&cs3user.GetUserResponse{
			Status: status.NewNotFound(ctx, "not found"),
		}, nil)
		client.On("GetUserByClaim", mock.Anything, mock.Anything).Return(&cs3user.GetUserByClaimResponse{
			Status: status.NewNotFound(ctx, "not found"),
		}, nil)

		// for public access
		client.On("Authenticate", mock.Anything, mock.MatchedBy(func(req *cs3gateway.AuthenticateRequest) bool {
			return req.Type == "publicshares" &&
				strings.HasPrefix(req.ClientId, "tokenfor") &&
				strings.HasPrefix(req.ClientSecret, "signature||")
		})).Return(&cs3gateway.AuthenticateResponse{
			Status: status.NewOK(ctx),
			User:   user,
			Token:  "jwt",
		}, nil)
		client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
			return req.Ref.ResourceId.StorageId == utils.PublicStorageProviderID &&
				req.Ref.ResourceId.SpaceId == utils.PublicStorageSpaceID &&
				req.Ref.ResourceId.OpaqueId == "tokenforfile"
		})).Return(&cs3storageprovider.StatResponse{
			Status: status.NewOK(ctx),
			Info: &cs3storageprovider.ResourceInfo{
				Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_FILE,
			},
		}, nil)
		client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
			return req.Ref.ResourceId.StorageId == utils.PublicStorageProviderID &&
				req.Ref.ResourceId.SpaceId == utils.PublicStorageSpaceID &&
				req.Ref.ResourceId.OpaqueId == "tokenforfolder"
		})).Return(&cs3storageprovider.StatResponse{
			Status: status.NewOK(ctx),
			Info: &cs3storageprovider.ResourceInfo{
				Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
			},
		}, nil)
	})

	Describe("NewHandler", func() {
		It("returns a handler", func() {
			Expect(handler).ToNot(BeNil())
		})
	})

	Context("When a default space is used", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				Expect(rr).To(HaveHTTPBody(""), "Body must be empty")

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", http.StatusNoContent),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", http.StatusNoContent),
		)
	})
})
