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
	"path"

	"google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("storage providers", func() {
	var (
		provider string
		revad    *Revad

		ctx           context.Context
		serviceClient storagep.ProviderAPIClient
	)

	BeforeEach(func() {
		var err error
		serviceClient, err = pool.GetStorageProviderServiceClient(grpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		// Add auth token
		user := &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
		}
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user)
		Expect(err).ToNot(HaveOccurred())
		ctx = token.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, t)
		ctx = ruser.ContextSetUser(ctx, user)

		revad, err = startRevad(path.Join("fixtures", "storageprovider-"+provider+".toml"))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		revad.Cleanup()
	})

	assertCreateHomeResponses := func() {
		It("creates a home directory", func() {
			homeRef := &storagep.Reference{
				Spec: &storagep.Reference_Path{Path: "/"},
			}
			res, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			_, err = serviceClient.CreateHome(ctx, &storagep.CreateHomeRequest{})
			Expect(err).ToNot(HaveOccurred())

			res, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	Describe("ocis", func() {
		BeforeEach(func() {
			provider = "ocis"
		})

		assertCreateHomeResponses()
	})
})
