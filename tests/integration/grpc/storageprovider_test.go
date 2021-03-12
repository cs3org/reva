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
	"bytes"
	"context"
	"io/ioutil"
	"os"

	"google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/fs/ocis"
	"github.com/cs3org/reva/pkg/storage/fs/owncloud"
	"github.com/cs3org/reva/pkg/token"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
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
		dependencies = map[string]string{}
		variables    = map[string]string{}
		revads       = map[string]*Revad{}

		ctx           context.Context
		serviceClient storagep.ProviderAPIClient
		user          = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
		}

		homeRef = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: "/"},
		}
		filePath = "/file"
		fileRef  = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: filePath},
		}
		versionedFilePath = "/versionedFile"
		versionedFileRef  = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: versionedFilePath},
		}
		subdirPath = "/subdir"
		subdirRef  = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: subdirPath},
		}
		sharesPath = "/Shares"
		sharesRef  = &storagep.Reference{
			Spec: &storagep.Reference_Path{Path: sharesPath},
		}
	)

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		// Add auth token
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user)
		Expect(err).ToNot(HaveOccurred())
		ctx = token.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, t)
		ctx = ruser.ContextSetUser(ctx, user)

		revads, err = startRevads(dependencies, variables)
		Expect(err).ToNot(HaveOccurred())
		serviceClient, err = pool.GetStorageProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
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

			ghRes, err := serviceClient.GetHome(ctx, &storagep.GetHomeRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ghRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
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

	assertFileVersions := func() {
		It("lists file versions", func() {
			listRes, err := serviceClient.ListFileVersions(ctx, &storagep.ListFileVersionsRequest{Ref: versionedFileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.Versions)).To(Equal(1))
			Expect(listRes.Versions[0].Size).To(Equal(uint64(1)))
		})

		It("restores a file version", func() {
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: versionedFileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.Size).To(Equal(uint64(2))) // second version contains 2 bytes

			listRes, err := serviceClient.ListFileVersions(ctx, &storagep.ListFileVersionsRequest{Ref: versionedFileRef})
			Expect(err).ToNot(HaveOccurred())
			restoreRes, err := serviceClient.RestoreFileVersion(ctx,
				&storagep.RestoreFileVersionRequest{
					Ref: versionedFileRef,
					Key: listRes.Versions[0].Key,
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(restoreRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: versionedFileRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(statRes.Info.Size).To(Equal(uint64(1))) // initial version contains 1 byte
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

	assertMove := func() {
		It("moves a directory", func() {
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			targetRef := &storagep.Reference{
				Spec: &storagep.Reference_Path{Path: "/new_subdir"},
			}
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
			By("deleting an item")
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the recycle items")
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			Expect(len(listRes.RecycleItems)).To(Equal(1))
			item := listRes.RecycleItems[0]
			Expect(item.Path).To(Equal(subdirPath))

			By("restoring a recycle item")
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			restoreRes, err := serviceClient.RestoreRecycleItem(ctx,
				&storagep.RestoreRecycleItemRequest{
					Ref: subdirRef,
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
			restoreRef := &storagep.Reference{
				Spec: &storagep.Reference_Path{Path: "/subdirRestored"},
			}
			By("deleting an item")
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing the recycle items")
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			Expect(len(listRes.RecycleItems)).To(Equal(1))
			item := listRes.RecycleItems[0]
			Expect(item.Path).To(Equal(subdirPath))

			By("restoring the item to a different location")
			statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: restoreRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			restoreRes, err := serviceClient.RestoreRecycleItem(ctx,
				&storagep.RestoreRecycleItemRequest{
					Ref:         subdirRef,
					Key:         item.Key,
					RestorePath: "/subdirRestored",
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(restoreRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: restoreRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		It("purges recycle items resources", func() {
			By("deleting an item")
			res, err := serviceClient.Delete(ctx, &storagep.DeleteRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("listing recycle items")
			listRes, err := serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.RecycleItems)).To(Equal(1))

			By("purging a recycle item")
			purgeRes, err := serviceClient.PurgeRecycle(ctx, &storagep.PurgeRecycleRequest{Ref: subdirRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(purgeRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			listRes, err = serviceClient.ListRecycle(ctx, &storagep.ListRecycleRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.RecycleItems)).To(Equal(0))
		})
	}

	assertReferences := func() {
		It("creates references", func() {
			listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: sharesRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
			Expect(len(listRes.Infos)).To(Equal(0))

			res, err := serviceClient.CreateReference(ctx, &storagep.CreateReferenceRequest{Path: "/Shares/reference", TargetUri: "scheme://target"})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: sharesRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(listRes.Infos)).To(Equal(1))
		})
	}

	assertMetadata := func() {
		It("sets and unsets metadata", func() {
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

	Describe("ocis", func() {
		BeforeEach(func() {
			dependencies = map[string]string{
				"storage": "storageprovider-ocis.toml",
			}
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
			assertMove()
			assertGrants()
			assertUploads()
			assertDownloads()
			assertRecycle()
			assertReferences()
			assertMetadata()
		})

		Context("with an existing file /versioned_file", func() {
			JustBeforeEach(func() {
				fs, err := ocis.New(map[string]interface{}{
					"root":        revads["storage"].TmpRoot,
					"enable_home": true,
				})
				Expect(err).ToNot(HaveOccurred())

				content1 := ioutil.NopCloser(bytes.NewReader([]byte("1")))
				content2 := ioutil.NopCloser(bytes.NewReader([]byte("22")))

				ctx := ruser.ContextSetUser(context.Background(), user)

				err = fs.CreateHome(ctx)
				Expect(err).ToNot(HaveOccurred())
				err = fs.Upload(ctx, versionedFileRef, content1)
				Expect(err).ToNot(HaveOccurred())
				err = fs.Upload(ctx, versionedFileRef, content2)
				Expect(err).ToNot(HaveOccurred())
			})

			assertFileVersions()
		})
	})

	Describe("owncloud", func() {
		BeforeEach(func() {
			dependencies = map[string]string{
				"users":   "userprovider-json.toml",
				"storage": "storageprovider-owncloud.toml",
			}

			redisAddress := os.Getenv("REDIS_ADDRESS")
			if redisAddress == "" {
				Fail("REDIS_ADDRESS not set")
			}
			variables = map[string]string{
				"redis_address": redisAddress,
			}
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
			assertMove()
			assertGrants()
			assertUploads()
			assertDownloads()
			assertRecycle()
			assertReferences()
			assertMetadata()
		})

		Context("with an existing file /versioned_file", func() {
			JustBeforeEach(func() {
				fs, err := owncloud.New(map[string]interface{}{
					"datadirectory":   revads["storage"].TmpRoot,
					"userprovidersvc": revads["users"].GrpcAddress,
					"enable_home":     true,
				})
				Expect(err).ToNot(HaveOccurred())

				content1 := ioutil.NopCloser(bytes.NewReader([]byte("1")))
				content2 := ioutil.NopCloser(bytes.NewReader([]byte("22")))

				ctx := ruser.ContextSetUser(context.Background(), user)

				err = fs.CreateHome(ctx)
				Expect(err).ToNot(HaveOccurred())
				err = fs.Upload(ctx, versionedFileRef, content1)
				Expect(err).ToNot(HaveOccurred())
				err = fs.Upload(ctx, versionedFileRef, content2)
				Expect(err).ToNot(HaveOccurred())
			})

			assertFileVersions()
		})
	})
})
