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

package nextcloud_test

import (
	"context"
	"os"

	"google.golang.org/grpc/metadata"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/auth/manager/nextcloud"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nextcloud", func() {
	var (
		ctx     context.Context
		options map[string]interface{}
		tmpRoot string
		user    = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "tester",
		}
	)

	BeforeEach(func() {
		var err error
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"root":         tmpRoot,
			"enable_home":  true,
			"share_folder": "/Shares",
		}

		ctx = context.Background()

		// Add auth token
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		scope, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user, scope)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, user)
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			_, err := nextcloud.New(options)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error)
	Describe("Authenticate", func() {
		It("calls the GetHome endpoint", func() {
			called := make([]string, 0)

			h := nextcloud.GetNextcloudServerMock(&called)
			mock, teardown := nextcloud.TestingHTTPClient(h)
			defer teardown()
			am, _ := nextcloud.NewAuthManager(&nextcloud.AuthManagerConfig{
				EndPoint: "http://mock.com/apps/sciencemesh/",
			}, mock)
			user, scope, err := am.Authenticate(ctx, "einstein", "relativity")
			Expect(err).ToNot(HaveOccurred())
			Expect(user).To(Equal(&userpb.User{
				Id: &userpb.UserId{
					Idp:      "some-idp",
					OpaqueId: "some-opaque-user-id",
					Type:     1,
				},
				Username:     "",
				Mail:         "",
				MailVerified: false,
				DisplayName:  "",
				Groups:       nil,
				Opaque:       nil,
				UidNumber:    0,
				GidNumber:    0,
			}))
			Expect(scope).To(Equal(map[string]*authpb.Scope{
				"user": {
					Resource: &types.OpaqueEntry{
						Decoder: "json",
						Value:   []byte("{\"resource_id\":{\"storage_id\":\"storage-id\",\"opaque_id\":\"opaque-id\"},\"path\":\"some/file/path.txt\"}"),
					},
					Role: 1,
				},
			}))
			Expect(len(called)).To(Equal(1))
			Expect(called[0]).To(Equal(`POST /apps/sciencemesh/~einstein/api/auth/Authenticate {"clientID":"einstein","clientSecret":"relativity"}`))
		})
	})

})
