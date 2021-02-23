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
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("storage providers", func() {
	var (
		dependencies map[string]string
		revads       map[string]*Revad

		ctx           context.Context
		serviceClient storagep.ProviderAPIClient

		homeRef    *storagep.Reference
		filePath   string
		fileRef    *storagep.Reference
		subdirPath string
		subdirRef  *storagep.Reference
	)

	BeforeEach(func() {
		homeRef = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: "/"},
		}
		filePath = "/file"
		fileRef = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: filePath},
		}
		subdirPath = "/subdir"
		subdirRef = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: subdirPath},
		}
		revads = map[string]*Revad{}
		dependencies = map[string]string{}
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
		serviceClient, err = pool.GetStorageProviderServiceClient(revad.GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		revad.Cleanup()
	})

	assertCreateHome := func() {
		It("creates a home directory", func() {
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err := serviceClient.CreateHome(ctx, &storagep.CreateHomeRequest{})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	assertCreateContainer := func() {
		It("creates a new directory", func() {
			newRef := &storagep.Reference{
				Spec: &storagep.Reference_Path{Path: "/newdir"},
			}

			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: newRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err := serviceClient.CreateContainer(ctx, &storagep.CreateContainerRequest{Ref: newRef})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: newRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	assertListContainer := func() {
		It("lists a directory", func() {
			listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.Infos)).To(Equal(1))

			info := listRes.Infos[0]
			Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
			Expect(info.Path).To(Equal(subdirPath))
			Expect(info.Owner.OpaqueId).To(Equal("f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"))
		})
	}

	assertDelete := func() {
		It("deletes a directory", func() {
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
		})
	}

	assertGetPath := func() {
		It("gets the path to an ID", func() {
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())

			res, err := serviceClient.GetPath(ctx, &storagep.GetPathRequest{ResourceId: statRes.Info.Id})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Path).To(Equal(subdirPath))
		})
	}

	assertGrants := func() {
		It("lists, adds and removes grants", func() {
			By("there are no grants initially")
			listRes, err := serviceClient.ListGrants(ctx, &storagep.ListGrantsRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(listRes.Grants)).To(Equal(0))

			By("adding a grant")
			grant := &storagep.Grant{
				Grantee: &storagep.Grantee{
					Type: storagep.GranteeType_GRANTEE_TYPE_USER,
					Id: &storagep.Grantee_UserId{
						UserId: &userpb.UserId{
							OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
						},
					},
				},
				Permissions: &storagep.ResourcePermissions{
					Stat:   true,
					Move:   true,
					Delete: false,
				},
			}
			addRes, err := serviceClient.AddGrant(ctx, &storagep.AddGrantRequest{Ref: subdirRef, Grant: grant})
			Expect(err).ToNot(HaveOccurred())
			Expect(addRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the new grant")
			listRes, err = serviceClient.ListGrants(ctx, &storagep.ListGrantsRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(listRes.Grants)).To(Equal(1))
			readGrant := listRes.Grants[0]
			Expect(readGrant.Permissions.Stat).To(BeTrue())
			Expect(readGrant.Permissions.Move).To(BeTrue())
			Expect(readGrant.Permissions.Delete).To(BeFalse())

			By("deleting a grant")
			delRes, err := serviceClient.RemoveGrant(ctx, &storagep.RemoveGrantRequest{Ref: subdirRef, Grant: readGrant})
			Expect(err).ToNot(HaveOccurred())
			Expect(delRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("the grant is gone")
			listRes, err = serviceClient.ListGrants(ctx, &storagep.ListGrantsRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(listRes.Grants)).To(Equal(0))
		})
	}

	assertUploads := func() {
		It("returns upload URLs for simple and tus", func() {
			res, err := serviceClient.InitiateFileUpload(ctx, &storagep.InitiateFileUploadRequest{Ref: fileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(res.Protocols)).To(Equal(2))
		})
	}

	assertDownloads := func() {
		It("returns 'simple' download URLs", func() {
			res, err := serviceClient.InitiateFileDownload(ctx, &storagep.InitiateFileDownloadRequest{Ref: fileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(res.Protocols)).To(Equal(1))
		})
	}

	assertRecycle := func() {
		It("lists and restores resources", func() {
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{})
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			Expect(len(listRes.RecycleItems)).To(Equal(1))
			item := listRes.RecycleItems[0]
			Expect(item.Path).To(Equal(subdirPath))
		})
	}

	Describe("ocis", func() {
		BeforeEach(func() {
			provider = "ocis"
		})

		assertCreateHome()

		Context("with a home and a subdirectory", func() {
			JustBeforeEach(func() {
				res, err := serviceClient.CreateHome(ctx, &storagep.CreateHomeRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				subdirRes, err := serviceClient.CreateContainer(ctx, &storagep.CreateContainerRequest{Ref: subdirRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(subdirRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})

			assertCreateContainer()
			assertListContainer()
			assertGetPath()
			assertDelete()
			assertGrants()
			assertUploads()
			assertDownloads()
			assertRecycle()
		})
	})
})
