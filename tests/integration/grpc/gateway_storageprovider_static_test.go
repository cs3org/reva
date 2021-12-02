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

package grpc_test

import (
	"context"
	"os"
	"path"

	"google.golang.org/grpc/metadata"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// This test suite tests the gprc gateway interface
//
// It uses the `startRevads` helper to spawn the according reva daemon and
// other dependencies like a userprovider if needed.
// It also sets up an authenticated context and a service client to the storage
// provider to be used in the assertion functions.
var _ = Describe("gateway using a static registry and a shard setup", func() {
	var (
		dependencies = map[string]string{}
		revads       = map[string]*Revad{}

		ctx           context.Context
		variables     map[string]string
		serviceClient gateway.GatewayAPIClient
		user          = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:39000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "marie",
		}
		homeRef = &storagep.Reference{Path: "/home"}
	)

	BeforeEach(func() {
		dependencies = map[string]string{
			"gateway":  "gateway-static.toml",
			"users":    "userprovider-json.toml",
			"storage":  "storageprovider-owncloud.toml",
			"storage2": "storageprovider-owncloud.toml",
		}
		redisAddress := os.Getenv("REDIS_ADDRESS")
		if redisAddress == "" {
			Fail("REDIS_ADDRESS not set")
		}
		variables = map[string]string{
			"redis_address": redisAddress,
		}
	})

	JustBeforeEach(func() {
		var err error
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

		revads, err = startRevads(dependencies, variables)
		Expect(err).ToNot(HaveOccurred())
		Expect(revads["gateway"]).ToNot(BeNil())
		serviceClient, err = pool.GetGatewayServiceClient(revads["gateway"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
	})

	It("creates a home directory on the correct provider", func() {
		statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
		Expect(err).ToNot(HaveOccurred())
		Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

		res, err := serviceClient.CreateHome(ctx, &storagep.CreateHomeRequest{})
		Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		Expect(err).ToNot(HaveOccurred())

		statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
		Expect(err).ToNot(HaveOccurred())
		Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		fi, err := os.Stat(path.Join(revads["storage2"].TmpRoot, user.Id.OpaqueId, "files"))
		Expect(err).ToNot(HaveOccurred())
		Expect(fi.IsDir()).To(BeTrue())
		_, err = os.Stat(path.Join(revads["storage"].TmpRoot, user.Id.OpaqueId, "files"))
		Expect(err).To(HaveOccurred())

		ghRes, err := serviceClient.GetHome(ctx, &storagep.GetHomeRequest{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ghRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
	})
})
