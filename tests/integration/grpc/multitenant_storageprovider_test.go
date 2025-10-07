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

	"google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/auth/scope"
	"github.com/opencloud-eu/reva/v2/pkg/conversions"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	jwt "github.com/opencloud-eu/reva/v2/pkg/token/manager/jwt"
	"github.com/opencloud-eu/reva/v2/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This test suite tests the gprc storageprovider interface using different
// storage backends
//
// It uses the `startRevads` helper to spawn the according reva daemon and
// other dependencies like a userprovider if needed.
// It also sets up an authenticated context and a service client to the storage
// provider to be used in the assertion functions.
var _ = Describe("storage providers", func() {
	var (
		dependencies = []RevadConfig{}
		variables    = map[string]string{}
		revads       = map[string]*Revad{}

		ctx            context.Context
		spacesClient   storagep.SpacesAPIClient
		providerClient storagep.ProviderAPIClient
		user           = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
				TenantId: "tenantid",
			},
			Username: "einstein",
		}
		sameTenantUserID = &userpb.UserId{
			Idp:      "0.0.0.0:19000",
			OpaqueId: "another-user",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
			TenantId: "tenantid",
		}
		differentTenantUserID = &userpb.UserId{
			Idp:      "0.0.0.0:19000",
			OpaqueId: "yet-another-user",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
			TenantId: "different-tenantid",
		}
	)

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		dependencies = []RevadConfig{
			{
				Name: "storage", Config: "storageprovider-multitenant.toml",
			},
			{
				Name: "permissions", Config: "permissions-opencloud-ci.toml",
			},
		}

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
		spacesClient, err = pool.GetSpacesProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
		providerClient, err = pool.GetStorageProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentSpecReport().Failed())).To(Succeed())
		}
	})

	It("create a project space and add members from different tenants", func() {
		By("creating a project space")
		res, err := spacesClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
			Owner: user,
			Type:  "project",
			Name:  user.Id.OpaqueId,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		By("adding a grant for a user from the same tenant")
		opaque := &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				"spacegrant": {},
			},
		}
		utils.AppendPlainToOpaque(opaque, "spacetype", "project")
		addGrantReq := &storagep.AddGrantRequest{
			Opaque: opaque,
			Ref: &storagep.Reference{
				ResourceId: res.StorageSpace.Root,
			},
			Grant: &storagep.Grant{
				Grantee: &storagep.Grantee{
					Type: storagep.GranteeType_GRANTEE_TYPE_USER,
					Id:   &storagep.Grantee_UserId{UserId: sameTenantUserID},
				},
				Permissions: conversions.NewSpaceEditorRole().CS3ResourcePermissions(),
			},
		}
		agres, err := providerClient.AddGrant(ctx, addGrantReq)
		Expect(err).ToNot(HaveOccurred())
		Expect(agres.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		By("adding a grant for a user from a different tenant should fail")
		addGrantReq.Grant.Grantee.Id = &storagep.Grantee_UserId{UserId: differentTenantUserID}
		agres, err = providerClient.AddGrant(ctx, addGrantReq)
		Expect(err).ToNot(HaveOccurred())
		Expect(agres.Status.Code).To(Equal(rpcv1beta1.Code_CODE_PERMISSION_DENIED))
		Expect(agres.Status.Message).To(ContainSubstring("cannot add grant for user from different tenant"))

		By("createing a folder in the project space as owner")
		mkdirRes, err := providerClient.CreateContainer(ctx, &storagep.CreateContainerRequest{
			Ref: &storagep.Reference{
				ResourceId: res.StorageSpace.Root,
				Path:       "./folder1",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(mkdirRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		By("adding a grant for a user from the same tenant to the folder")
		addGrantReq.Ref = &storagep.Reference{
			ResourceId: res.StorageSpace.Root,
			Path:       "./folder1",
		}
		addGrantReq.Grant.Grantee.Id = &storagep.Grantee_UserId{UserId: sameTenantUserID}
		addGrantReq.Grant.Permissions = conversions.NewEditorRole().CS3ResourcePermissions()
		addGrantReq.Opaque = nil
		agres, err = providerClient.AddGrant(ctx, addGrantReq)
		Expect(err).ToNot(HaveOccurred())
		Expect(agres.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		By("adding a grant for a user from a different tenant to the folder")
		addGrantReq.Ref = &storagep.Reference{
			ResourceId: res.StorageSpace.Root,
			Path:       "./folder1",
		}
		addGrantReq.Grant.Grantee.Id = &storagep.Grantee_UserId{UserId: differentTenantUserID}
		addGrantReq.Grant.Permissions = conversions.NewEditorRole().CS3ResourcePermissions()
		addGrantReq.Opaque = nil
		agres, err = providerClient.AddGrant(ctx, addGrantReq)
		Expect(err).ToNot(HaveOccurred())
		Expect(agres.Status.Code).To(Equal(rpcv1beta1.Code_CODE_PERMISSION_DENIED))
	})
})
