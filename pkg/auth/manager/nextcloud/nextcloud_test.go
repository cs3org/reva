// Copyright 2018-2023 CERN
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
	"fmt"
	"os"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/manager/nextcloud"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/metadata"
)

func setUpNextcloudServer() (*nextcloud.Manager, *[]string, func()) {
	var conf *nextcloud.AuthManagerConfig

	ncHost := os.Getenv("NEXTCLOUD")
	if len(ncHost) == 0 {
		conf = &nextcloud.AuthManagerConfig{
			EndPoint: "http://mock.com/apps/sciencemesh/",
			MockHTTP: true,
		}
		nc, _ := nextcloud.NewAuthManager(conf)
		called := make([]string, 0)
		h := nextcloud.GetNextcloudServerMock(&called)
		mock, teardown := nextcloud.TestingHTTPClient(h)
		nc.SetHTTPClient(mock)
		return nc, &called, teardown
	}
	conf = &nextcloud.AuthManagerConfig{
		EndPoint: ncHost + "/apps/sciencemesh/",
		MockHTTP: false,
	}
	nc, _ := nextcloud.NewAuthManager(conf)
	return nc, nil, func() {}
}

func checkCalled(called *[]string, expected string) {
	if called == nil {
		return
	}
	Expect(len(*called)).To(Equal(1))
	Expect((*called)[0]).To(Equal(expected))
}

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

		options = map[string]interface{}{
			"endpoint":  "http://mock.com/apps/sciencemesh/",
			"mock_http": true,
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
			fmt.Println(options)
			_, err := nextcloud.New(options)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error)
	Describe("Authenticate", func() {
		It("calls the GetHome endpoint", func() {
			am, called, teardown := setUpNextcloudServer()
			defer teardown()

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
			checkCalled(called, `POST /apps/sciencemesh/~einstein/api/auth/Authenticate {"clientID":"einstein","clientSecret":"relativity"}`)
		})
	})

})
