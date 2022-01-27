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
	"time"

	"google.golang.org/grpc/metadata"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/ocis"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// This test suite tests the gprc gateway interface
//
// It uses the `startRevads` helper to spawn the according reva daemon and
// other dependencies like a userprovider if needed.
// It also sets up an authenticated context and a service client to the storage
// provider to be used in the assertion functions.
var _ = Describe("gateway", func() {
	var (
		dependencies = map[string]string{}
		variables    = map[string]string{}
		revads       = map[string]*Revad{}

		ctx           context.Context
		serviceClient gateway.GatewayAPIClient
		user          = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:39000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}
		homeRef = &storagep.Reference{
			ResourceId: &storagep.ResourceId{
				StorageId: user.Id.OpaqueId,
				OpaqueId:  user.Id.OpaqueId,
			},
			Path: ".",
		}

		infos2Etags = func(infos []*storagep.ResourceInfo) map[string]string {
			etags := map[string]string{}
			for _, info := range infos {
				etags[info.Path] = info.Etag
			}
			return etags
		}
		infos2Paths = func(infos []*storagep.ResourceInfo) []string {
			paths := []string{}
			for _, info := range infos {
				paths = append(paths, info.Path)
			}
			return paths
		}
	)

	BeforeEach(func() {
		dependencies = map[string]string{
			"gateway":     "gateway.toml",
			"users":       "userprovider-json.toml",
			"storage":     "storageprovider-ocis.toml",
			"storage2":    "storageprovider-ocis.toml",
			"permissions": "permissions-ocis-ci.toml",
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

	Context("with a sharded projects directory", func() {
		var (
			shard1Fs    storage.FS
			shard1Space *storagep.StorageSpace
			shard2Fs    storage.FS
			projectsRef = &storagep.Reference{Path: "/projects"}

			getProjectsEtag = func() string {
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/"}})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(1))
				return listRes.Infos[0].Etag
			}
		)

		BeforeEach(func() {
			dependencies = map[string]string{
				"gateway":     "gateway-sharded.toml",
				"users":       "userprovider-json.toml",
				"homestorage": "storageprovider-ocis.toml",
				"storage":     "storageprovider-ocis.toml",
				"storage2":    "storageprovider-ocis.toml",
				"permissions": "permissions-ocis-ci.toml",
			}
		})

		JustBeforeEach(func() {
			var err error
			shard1Fs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage"].StorageRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())
			res, err := shard1Fs.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Type:  "project",
				Name:  "a - project",
				Owner: user,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			shard1Space = res.StorageSpace

			err = helpers.Upload(ctx,
				shard1Fs,
				&storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: shard1Space.Id.OpaqueId}, Path: "/file.txt"},
				[]byte("1"),
			)
			Expect(err).ToNot(HaveOccurred())

			shard2Fs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage"].StorageRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())
			res, err = shard2Fs.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Type:  "project",
				Name:  "z - project",
				Owner: user,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		Describe("ListContainer", func() {
			// Note: The Gateway doesn't merge any lists any more. This needs to be done by the client
			// TODO: Move the tests to a place where they can actually test something
			PIt("merges the lists of both shards", func() {
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: projectsRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(infos2Paths(listRes.Infos)).To(ConsistOf([]string{"/projects/a - project", "/projects/z - project"}))
			})

			PIt("propagates the etags from both shards", func() {
				rootEtag := getProjectsEtag()

				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: projectsRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				etags := infos2Etags(listRes.Infos)
				Expect(etags["/projects/a - project"]).ToNot(BeNil())
				Expect(etags["/projects/z - project"]).ToNot(BeNil())

				By("creating a new file")
				err = helpers.Upload(ctx, shard1Fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: shard1Space.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: projectsRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etags2 := infos2Etags(listRes.Infos)
				Expect(etags2["/projects/a - project"]).ToNot(Equal(etags["/projects/a - project"]))
				Expect(etags2["/projects/z - project"]).To(Equal(etags["/projects/z - project"]))

				rootEtag2 := getProjectsEtag()
				Expect(rootEtag2).ToNot(Equal(rootEtag))

				By("updating an existing file")
				err = helpers.Upload(ctx, shard1Fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: shard1Space.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("12345678901"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: projectsRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etags3 := infos2Etags(listRes.Infos)
				Expect(etags3["/projects/a - project"]).ToNot(Equal(etags2["/projects/a - project"]))
				Expect(etags3["/projects/z - project"]).To(Equal(etags2["/projects/z - project"]))

				rootEtag3 := getProjectsEtag()
				Expect(rootEtag3).ToNot(Equal(rootEtag2))

				By("creating a directory")
				err = shard1Fs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: shard1Space.Id.OpaqueId}, Path: "/newdirectory"})
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: projectsRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etags4 := infos2Etags(listRes.Infos)
				Expect(etags4["/projects/a - project"]).ToNot(Equal(etags3["/projects/a - project"]))
				Expect(etags4["/projects/z - project"]).To(Equal(etags3["/projects/z - project"]))

				rootEtag4 := getProjectsEtag()
				Expect(rootEtag4).ToNot(Equal(rootEtag3))
			})

			It("places new spaces in the correct shard", func() {
				createRes, err := serviceClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"path": {
								Decoder: "plain",
								Value:   []byte("/projects"),
							},
						},
					},
					Owner: user,
					Type:  "project",
					Name:  "o - project",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(createRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				space := createRes.StorageSpace

				ref := &storagep.Reference{
					ResourceId: space.Root,
					Path:       ".",
				}

				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: ref})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				_, err = os.Stat(path.Join(revads["storage"].StorageRoot, "/spaces/project", space.Id.OpaqueId))
				Expect(err).To(HaveOccurred())
				_, err = os.Stat(path.Join(revads["storage2"].StorageRoot, "/spaces/project", space.Id.OpaqueId))
				Expect(err).ToNot(HaveOccurred())
			})

			PIt("deletes spaces", func() {})

			It("lists individual project spaces", func() {
				By("trying to list a non-existent space")
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: &storagep.Reference{
					ResourceId: &storagep.ResourceId{
						StorageId: "does-not-exist",
						OpaqueId:  "neither-supposed-to-exist",
					},
					Path: ".",
				}})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

				By("listing an existing space")
				listRes, err = serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: &storagep.Reference{ResourceId: shard1Space.Root, Path: "."}})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(2))
				paths := []string{}
				for _, i := range listRes.Infos {
					paths = append(paths, i.Path)
				}
				Expect(paths).To(ConsistOf([]string{"file.txt", ".space"}))
			})

		})
	})

	Context("with a basic user storage", func() {
		var (
			fs              storage.FS
			embeddedFs      storage.FS
			homeSpace       *storagep.StorageSpace
			embeddedSpace   *storagep.StorageSpace
			embeddedSpaceID string
			embeddedRef     *storagep.Reference
		)

		BeforeEach(func() {
			dependencies = map[string]string{
				"gateway":     "gateway.toml",
				"users":       "userprovider-json.toml",
				"storage":     "storageprovider-ocis.toml",
				"storage2":    "storageprovider-ocis.toml",
				"permissions": "permissions-ocis-ci.toml",
			}
		})

		JustBeforeEach(func() {
			var err error
			fs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage"].StorageRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"gateway_addr":        revads["gateway"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())

			r, err := serviceClient.CreateHome(ctx, &storagep.CreateHomeRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(r.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			spaces, err := fs.ListStorageSpaces(ctx, []*storagep.ListStorageSpacesRequest_Filter{})
			Expect(err).ToNot(HaveOccurred())
			homeSpace = spaces[0]

			err = helpers.Upload(ctx,
				fs,
				&storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/file.txt"},
				[]byte("1"),
			)
			Expect(err).ToNot(HaveOccurred())

			embeddedFs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage2"].StorageRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())
			res, err := serviceClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Type:  "project",
				Name:  "embedded project",
				Owner: user,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			embeddedSpace = res.StorageSpace
			embeddedRef = &storagep.Reference{
				ResourceId: &storagep.ResourceId{
					StorageId: embeddedSpace.Id.OpaqueId,
					OpaqueId:  embeddedSpace.Id.OpaqueId,
				},
				Path: ".", //  path.Join(homeRef.Path, "Projects", embeddedSpace.Id.OpaqueId),
			}
			err = helpers.Upload(ctx,
				embeddedFs,
				&storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/embedded.txt"},
				[]byte("22"),
			)
			Expect(err).ToNot(HaveOccurred())
			embeddedSpaceID = embeddedSpace.Id.OpaqueId
		})

		Describe("ListContainer", func() {
			It("lists the root", func() {
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(2))

				var fileInfo *storagep.ResourceInfo
				// var embeddedInfo *storagep.ResourceInfo
				for _, i := range listRes.Infos {
					if i.Path == "file.txt" {
						fileInfo = i
					} // else if i.Path == "Projects" {
					// embeddedInfo = i
					// }

				}
				Expect(fileInfo).ToNot(BeNil())
				Expect(fileInfo.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(fileInfo.Path).To(Equal("file.txt"))
				Expect(fileInfo.Size).To(Equal(uint64(1)))

				// Expect(embeddedInfo).ToNot(BeNil())
				// Expect(embeddedInfo.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				// Expect(embeddedInfo.Path).To(Equal("Projects"))
				// Expect(embeddedInfo.Size).To(Equal(uint64(2)))
			})

			PIt("lists the embedded project space", func() {
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: embeddedRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(2))

				var embeddedInfo *storagep.ResourceInfo
				for _, i := range listRes.Infos {
					if i.Path == path.Join(embeddedRef.Path, "embedded.txt") {
						embeddedInfo = i
					}

				}
				Expect(embeddedInfo).ToNot(BeNil())
				Expect(embeddedInfo.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(embeddedInfo.Path).To(Equal(path.Join(embeddedRef.Path, "embedded.txt")))
				Expect(embeddedInfo.Size).To(Equal(uint64(2)))
			})
		})

		Describe("Stat", func() {
			It("stats the root", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				info := statRes.Info
				Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(info.Path).To(Equal(user.Id.OpaqueId))
				Expect(info.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))

				// TODO: size aggregating is done by the client now - so no chance testing that here
				// Expect(info.Size).To(Equal(uint64(3))) // home: 1, embedded: 2
			})

			It("stats the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: embeddedRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				info := statRes.Info
				Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(info.Path).To(Equal(embeddedSpaceID))
				Expect(info.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(info.Size).To(Equal(uint64(2)))
			})

			PIt("propagates Sizes from within the root space", func() {
				// TODO: this cannot work atm as the propagation is not done by the gateway anymore
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(3)))

				By("Uploading a new file")
				err = helpers.Upload(ctx, fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(13)))

				By("Uploading a new file into a subdir")
				err = fs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())
				err = helpers.Upload(ctx, fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, []byte("1234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(23)))

				By("Updating an existing file")
				err = helpers.Upload(ctx, fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, []byte("12345678901234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(33)))
			})

			PIt("propagates Sizes from within the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(3)))

				By("Uploading a new file")
				err = helpers.Upload(ctx, embeddedFs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(13)))

				By("Uploading a new file into a subdir")
				err = embeddedFs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())
				err = helpers.Upload(ctx, embeddedFs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, []byte("1234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(23)))

				By("Updating an existing file")
				err = helpers.Upload(ctx, embeddedFs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, []byte("12345678901234567890"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(33)))
			})

			It("propagates Etags from within the root space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etag := statRes.Info.Etag

				By("Uploading a new file")
				err = helpers.Upload(ctx, fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag := statRes.Info.Etag

				Expect(newEtag).ToNot(Equal(etag))

				By("Creating a new dir")
				err = fs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag2 := statRes.Info.Etag

				Expect(newEtag2).ToNot(Equal(newEtag))

				By("Updating an existing file")
				err = helpers.Upload(ctx, fs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/file.txt"}, []byte("2"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag3 := statRes.Info.Etag

				Expect(newEtag3).ToNot(Equal(newEtag2))
			})

			PIt("propagates Etags from within the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etag := statRes.Info.Etag

				By("Uploading a new file")
				err = helpers.Upload(ctx, embeddedFs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag := statRes.Info.Etag

				Expect(newEtag).ToNot(Equal(etag))

				By("Creating a new dir")
				err = embeddedFs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag2 := statRes.Info.Etag

				Expect(newEtag2).ToNot(Equal(newEtag))

				By("Updating an existing file")
				err = helpers.Upload(ctx, embeddedFs, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, []byte("1"))
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(time.Second) // cache must expire
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag3 := statRes.Info.Etag

				Expect(newEtag3).ToNot(Equal(newEtag2))
			})
		})

		Describe("Move", func() {
			It("moves a directory", func() {
				sourceRef := &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId, OpaqueId: homeSpace.Id.OpaqueId}, Path: "./source"}
				targetRef := &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId, OpaqueId: homeSpace.Id.OpaqueId}, Path: "./destination"}
				dstRef := &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId, OpaqueId: homeSpace.Id.OpaqueId}, Path: "./destination/source"}

				err := fs.CreateDir(ctx, sourceRef)
				Expect(err).ToNot(HaveOccurred())
				err = fs.CreateDir(ctx, targetRef)
				Expect(err).ToNot(HaveOccurred())

				mvRes, err := serviceClient.Move(ctx, &storagep.MoveRequest{Source: sourceRef, Destination: dstRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(mvRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: sourceRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: dstRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})
		})
	})
})
