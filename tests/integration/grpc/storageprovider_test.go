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
	"github.com/cs3org/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/fs/nextcloud"
	"github.com/cs3org/reva/v2/pkg/storage/fs/ocis"
	jwt "github.com/cs3org/reva/v2/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v2/tests/helpers"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func ref(provider string, path string) *storagep.Reference {
	r := &storagep.Reference{
		Path: path,
	}
	if provider == "ocis" {
		r.ResourceId = &storagep.ResourceId{
			SpaceId:  "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
		}
	}
	return r
}

func createFS(provider string, revads map[string]*Revad) (storage.FS, error) {
	conf := make(map[string]interface{})
	var f func(map[string]interface{}) (storage.FS, error)
	switch provider {
	case "ocis":
		conf["root"] = revads["storage"].StorageRoot
		conf["permissionssvc"] = revads["permissions"].GrpcAddress
		f = ocis.New
	case "nextcloud":
		conf["endpoint"] = "http://localhost:8080/apps/sciencemesh/"
		conf["mock_http"] = true
		f = nextcloud.New
	}
	return f(conf)
}

// This test suite tests the gprc storageprovider interface using different
// storage backends
//
// It uses the `startRevads` helper to spawn the according reva daemon and
// other dependencies like a userprovider if needed.
// It also sets up an authenticated context and a service client to the storage
// provider to be used in the assertion functions.
var _ = Describe("storage providers", func() {
	var (
		dependencies = map[string]string{}
		variables    = map[string]string{}
		revads       = map[string]*Revad{}

		ctx           context.Context
		serviceClient storagep.ProviderAPIClient
		user          = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}

		homePath           = "/"
		filePath           = "/file"
		versionedFilePath  = "/versionedFile"
		subdirPath         = "/subdir"
		subdirRestoredPath = "/subdirRestored"
		sharesPath         = "/Shares"
	)

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
		serviceClient, err = pool.GetStorageProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentSpecReport().Failed())).To(Succeed())
		}
	})

	assertCreateHome := func(provider string) {
		It("creates a home directory", func() {
			homeRef := ref(provider, homePath)
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err := serviceClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Owner: user,
				Type:  "personal",
				Name:  user.Id.OpaqueId,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: ref(provider, homePath)})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			// ghRes, err := serviceClient.GetHome(ctx, &storagep.GetHomeRequest{})
			// Expect(err).ToNot(HaveOccurred())
			// Expect(ghRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	assertCreateContainer := func(provider string) {
		It("creates a new directory", func() {
			newRef := ref(provider, "/newdir")
			_, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: newRef})
			Expect(err).ToNot(HaveOccurred())
			// Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err := serviceClient.CreateContainer(ctx, &storagep.CreateContainerRequest{Ref: newRef})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: newRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	assertListContainer := func(provider string) {
		It("lists a directory", func() {
			listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: ref(provider, homePath)})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			switch provider {
			case "ocis":
				Expect(len(listRes.Infos)).To(Equal(1)) // subdir
			case "nextcloud":
				Expect(len(listRes.Infos)).To(Equal(1)) // subdir
			default:
				Fail("unknown provider")
			}

			for _, info := range listRes.Infos {
				switch info.Path {
				default:
					Fail("unknown path: " + info.Path)
				case "/.space":
					Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
				case subdirPath:
					Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
					Expect(info.Owner.OpaqueId).To(Equal("f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"))

				}
			}
		})
	}

	assertFileVersions := func(provider string) {
		It("lists file versions", func() {
			listRes, err := serviceClient.ListFileVersions(ctx, &storagep.ListFileVersionsRequest{Ref: ref(provider, versionedFilePath)})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.Versions)).To(Equal(1))
			Expect(listRes.Versions[0].Size).To(Equal(uint64(1)))
		})

		// FIXME flaky test?!?
		It("restores a file version", func() {
			vRef := ref(provider, versionedFilePath)
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: vRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.Size).To(Equal(uint64(2))) // second version contains 2 bytes

			listRes, err := serviceClient.ListFileVersions(ctx, &storagep.ListFileVersionsRequest{Ref: vRef})
			Expect(err).ToNot(HaveOccurred())
			restoreRes, err := serviceClient.RestoreFileVersion(ctx,
				&storagep.RestoreFileVersionRequest{
					Ref: vRef,
					Key: listRes.Versions[0].Key,
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(restoreRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: vRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.Size).To(Equal(uint64(1))) // initial version contains 1 byte
		})
	}

	assertDelete := func(provider string) {
		It("deletes a directory", func() {
			subdirRef := ref(provider, subdirPath)
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

	assertMove := func(provider string) {
		It("moves a directory", func() {
			subdirRef := ref(provider, subdirPath)
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			targetRef := &storagep.Reference{ResourceId: subdirRef.ResourceId, Path: "/new_subdir"}
			res, err := serviceClient.Move(ctx, &storagep.MoveRequest{Source: subdirRef, Destination: targetRef})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: targetRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})
	}

	assertGetPath := func(provider string) {
		It("gets the path to an ID", func() {
			r := ref(provider, subdirPath)
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: r})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			res, err := serviceClient.GetPath(ctx, &storagep.GetPathRequest{ResourceId: statRes.Info.Id})
			Expect(err).ToNot(HaveOccurred())

			// TODO: FIXME both cases should work for all providers

			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			if provider != "nextcloud" {
				Expect(res.Path).To(Equal(subdirPath))
			}
		})
	}

	assertGrants := func(provider string) {
		It("lists, adds and removes grants", func() {
			By("there are no grants initially")
			subdirRef := ref(provider, subdirPath)
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

			By("updating the grant")
			grant.Permissions.Delete = true
			updateRes, err := serviceClient.UpdateGrant(ctx, &storagep.UpdateGrantRequest{Ref: subdirRef, Grant: grant})
			Expect(err).ToNot(HaveOccurred())
			Expect(updateRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the update grant")
			listRes, err = serviceClient.ListGrants(ctx, &storagep.ListGrantsRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(listRes.Grants)).To(Equal(1))
			readGrant = listRes.Grants[0]
			Expect(readGrant.Permissions.Stat).To(BeTrue())
			Expect(readGrant.Permissions.Move).To(BeTrue())
			Expect(readGrant.Permissions.Delete).To(BeTrue())

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

	assertUploads := func(provider string) {
		It("returns upload URLs for simple and tus", func() {
			fileRef := ref(provider, filePath)
			res, err := serviceClient.InitiateFileUpload(ctx, &storagep.InitiateFileUploadRequest{Ref: fileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(res.Protocols)).To(Equal(2))
		})
	}

	assertDownloads := func(provider string) {
		It("returns 'simple' download URLs", func() {
			fileRef := ref(provider, filePath)
			res, err := serviceClient.InitiateFileDownload(ctx, &storagep.InitiateFileDownloadRequest{Ref: fileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(res.Protocols)).To(Equal(1))
		})
	}

	assertRecycle := func(provider string) {
		It("lists and restores resources", func() {
			By("deleting an item")
			subdirRef := ref(provider, subdirPath)
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the recycle items")
			homeRef := ref(provider, homePath)
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			Expect(len(listRes.RecycleItems)).To(Equal(1))
			item := listRes.RecycleItems[0]
			Expect(item.Ref.Path).To(Equal(subdirPath))

			By("restoring a recycle item")
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			restoreRes, err := serviceClient.RestoreRecycleItem(ctx,
				&storagep.RestoreRecycleItemRequest{
					Ref: homeRef,
					Key: item.Key,
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(restoreRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		It("restores resources to a different location", func() {
			restoreRef := ref(provider, subdirRestoredPath)
			subdirRef := ref(provider, subdirPath)
			homeRef := ref(provider, homePath)

			By("deleting an item")
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the recycle items")
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			Expect(len(listRes.RecycleItems)).To(Equal(1))
			item := listRes.RecycleItems[0]
			Expect(item.Ref.Path).To(Equal(subdirPath))

			By("restoring the item to a different location")
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: restoreRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			restoreRes, err := serviceClient.RestoreRecycleItem(ctx,
				&storagep.RestoreRecycleItemRequest{
					Ref:        homeRef,
					Key:        item.Key,
					RestoreRef: restoreRef,
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(restoreRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: restoreRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		It("purges recycle items resources", func() {
			subdirRef := ref(provider, subdirPath)
			homeRef := ref(provider, homePath)

			By("deleting an item")
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing recycle items")
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.RecycleItems)).To(Equal(1))

			By("purging a recycle item")
			ref := listRes.RecycleItems[0].Ref
			ref.ResourceId = homeRef.ResourceId
			purgeRes, err := serviceClient.PurgeRecycle(ctx, &storagep.PurgeRecycleRequest{Ref: ref})
			Expect(err).ToNot(HaveOccurred())
			Expect(purgeRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			listRes, err = serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.RecycleItems)).To(Equal(0))
		})
	}

	assertReferences := func(provider string) {
		It("creates references", func() {
			if provider == "ocis" {
				// ocis can't create references like this
				return
			}

			sharesRef := ref(provider, sharesPath)
			listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: sharesRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
			Expect(len(listRes.Infos)).To(Equal(0))

			res, err := serviceClient.CreateReference(ctx, &storagep.CreateReferenceRequest{
				Ref: &storagep.Reference{
					Path: "/Shares/reference",
				},
				TargetUri: "scheme://target",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: sharesRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.Infos)).To(Equal(1))
		})
	}

	assertMetadata := func(provider string) {
		It("sets and unsets metadata", func() {
			subdirRef := ref(provider, subdirPath)
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.ArbitraryMetadata.Metadata["foo"]).To(BeEmpty())

			By("setting arbitrary metadata")
			samRes, err := serviceClient.SetArbitraryMetadata(ctx, &storagep.SetArbitraryMetadataRequest{
				Ref:               subdirRef,
				ArbitraryMetadata: &storagep.ArbitraryMetadata{Metadata: map[string]string{"foo": "bar"}},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(samRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.ArbitraryMetadata.Metadata["foo"]).To(Equal("bar"))

			By("unsetting arbitrary metadata")
			uamRes, err := serviceClient.UnsetArbitraryMetadata(ctx, &storagep.UnsetArbitraryMetadataRequest{
				Ref:                   subdirRef,
				ArbitraryMetadataKeys: []string{"foo"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(uamRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.ArbitraryMetadata.Metadata["foo"]).To(BeEmpty())
		})
	}

	assertLocking := func(provider string) {
		var (
			subdirRef = ref(provider, subdirPath)
			lock      = &storagep.Lock{
				Type:   storagep.LockType_LOCK_TYPE_EXCL,
				User:   user.Id,
				LockId: uuid.New().String(),
			}
		)
		It("locks, gets, refreshes and unlocks a lock", func() {
			lockRes, err := serviceClient.SetLock(ctx, &storagep.SetLockRequest{
				Ref:  subdirRef,
				Lock: lock,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(lockRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			getRes, err := serviceClient.GetLock(ctx, &storagep.GetLockRequest{
				Ref: subdirRef,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(getRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(getRes.Lock.Type).To(Equal(lock.Type))
			Expect(getRes.Lock.User).To(Equal(lock.User))
			Expect(getRes.Lock.LockId).To(Equal(lock.LockId))
			Expect(getRes.Lock.AppName).To(Equal(lock.AppName))
			Expect(getRes.Lock.Expiration).To(Equal(lock.Expiration))
			Expect(getRes.Lock.Opaque).To(Equal(lock.Opaque))

			refreshRes, err := serviceClient.RefreshLock(ctx, &storagep.RefreshLockRequest{
				Ref:  subdirRef,
				Lock: lock,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(refreshRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			unlockRes, err := serviceClient.Unlock(ctx, &storagep.UnlockRequest{
				Ref:  subdirRef,
				Lock: lock,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(unlockRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		Context("with a locked file", func() {
			JustBeforeEach(func() {
				lockRes, err := serviceClient.SetLock(ctx, &storagep.SetLockRequest{
					Ref:  subdirRef,
					Lock: lock,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(lockRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})

			It("removes the lock when unlocking", func() {
				delRes, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{
					Ref: subdirRef,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(delRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_PERMISSION_DENIED))

				unlockRes, err := serviceClient.Unlock(ctx, &storagep.UnlockRequest{
					Ref:  subdirRef,
					Lock: lock,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(unlockRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				delRes, err = serviceClient.Delete(ctx, &storagep.DeleteRequest{
					Ref: subdirRef,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(delRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			})

			// FIXME these tests are all wrong as they use the reference of a directory, but try to lock and upload a file
			Context("with the owner holding the lock", func() {
				It("can not initiate an upload that would overwrite a folder", func() {
					ulRes, err := serviceClient.InitiateFileUpload(ctx, &storagep.InitiateFileUploadRequest{
						Ref:    subdirRef,
						LockId: lock.LockId,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(ulRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_FAILED_PRECONDITION))
				})

				It("can delete the file", func() {
					delRes, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{
						Ref:    subdirRef,
						LockId: lock.LockId,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(delRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				})
			})
			Context("with the owner not holding the lock", func() {
				It("can only delete after unlocking the file", func() {
					delRes, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{
						Ref: subdirRef,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(delRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_PERMISSION_DENIED))
				})
			})

		})
	}

	suite := func(provider string, deps map[string]string) {
		Describe(provider, func() {
			BeforeEach(func() {
				dependencies = deps
				variables = map[string]string{
					"enable_home": "true",
				}
			})

			assertCreateHome(provider)

			Context("with a home and a subdirectory", func() {
				JustBeforeEach(func() {
					res, err := serviceClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
						Owner: user,
						Type:  "personal",
						Name:  user.Id.OpaqueId,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					subdirRes, err := serviceClient.CreateContainer(ctx, &storagep.CreateContainerRequest{Ref: ref(provider, subdirPath)})
					Expect(err).ToNot(HaveOccurred())
					Expect(subdirRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				})

				assertCreateContainer(provider)
				assertListContainer(provider)
				assertGetPath(provider)
				assertDelete(provider)
				assertMove(provider)
				assertGrants(provider)
				assertUploads(provider)
				assertDownloads(provider)
				assertRecycle(provider)
				assertReferences(provider)
				assertMetadata(provider)
				if provider == "ocis" {
					assertLocking(provider)
				} else {
					PIt("Locking implementation still pending for provider " + provider)
				}
			})

			Context("with an existing file /versioned_file", func() {
				JustBeforeEach(func() {
					fs, err := createFS(provider, revads)
					Expect(err).ToNot(HaveOccurred())

					content1 := []byte("1")
					content2 := []byte("22")

					vRef := ref(provider, versionedFilePath)
					if provider == "nextcloud" {
						vRef.ResourceId = &storagep.ResourceId{StorageId: user.Id.OpaqueId}
					}

					_, err = fs.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
						Owner: user,
						Type:  "personal",
					})
					Expect(err).ToNot(HaveOccurred())
					err = helpers.Upload(ctx, fs, vRef, content1)
					Expect(err).ToNot(HaveOccurred())
					err = helpers.Upload(ctx, fs, vRef, content2)
					Expect(err).ToNot(HaveOccurred())
				})

				assertFileVersions(provider)
			})
		})

	}

	suite("nextcloud", map[string]string{
		"storage": "storageprovider-nextcloud.toml",
	})

	suite("ocis", map[string]string{
		"storage":     "storageprovider-ocis.toml",
		"permissions": "permissions-ocis-ci.toml",
	})

})
