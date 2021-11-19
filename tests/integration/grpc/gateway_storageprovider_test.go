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
	"path"

	"google.golang.org/grpc/metadata"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/ocis"
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
		homeRef = &storagep.Reference{Path: "/users/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"}
	)

	BeforeEach(func() {
		dependencies = map[string]string{
			"gateway":  "gateway.toml",
			"users":    "userprovider-json.toml",
			"storage":  "storageprovider-ocis.toml",
			"storage2": "storageprovider-ocis.toml",
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

	Context("with a basic user storage", func() {
		var (
			fs            storage.FS
			embeddedFs    storage.FS
			homeSpace     *storagep.StorageSpace
			embeddedSpace *storagep.StorageSpace
			embeddedRef   *storagep.Reference
		)

		JustBeforeEach(func() {
			var err error
			fs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage"].TmpRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())
			err = fs.CreateHome(ctx)
			Expect(err).ToNot(HaveOccurred())

			spaces, err := fs.ListStorageSpaces(ctx, []*storagep.ListStorageSpacesRequest_Filter{}, nil)
			Expect(err).ToNot(HaveOccurred())
			homeSpace = spaces[0]

			err = fs.Upload(ctx,
				&storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/file.txt"},
				ioutil.NopCloser(bytes.NewReader([]byte("1"))))
			Expect(err).ToNot(HaveOccurred())

			embeddedFs, err = ocis.New(map[string]interface{}{
				"root":                revads["storage2"].TmpRoot,
				"userprovidersvc":     revads["users"].GrpcAddress,
				"enable_home":         true,
				"treesize_accounting": true,
				"treetime_accounting": true,
			})
			Expect(err).ToNot(HaveOccurred())
			res, err := embeddedFs.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Type:  "project",
				Name:  "embedded project",
				Owner: user,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			embeddedSpace = res.StorageSpace
			embeddedRef = &storagep.Reference{Path: path.Join(homeRef.Path, "Projects", embeddedSpace.Id.OpaqueId)}
			err = embeddedFs.Upload(ctx,
				&storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/embedded.txt"},
				ioutil.NopCloser(bytes.NewReader([]byte("22"))))
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("ListContainer", func() {
			It("lists the root", func() {
				listRes, err := serviceClient.ListContainer(ctx, &storagep.ListContainerRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(2))

				var fileInfo *storagep.ResourceInfo
				var embeddedInfo *storagep.ResourceInfo
				for _, i := range listRes.Infos {
					if i.Path == "/users/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c/file.txt" {
						fileInfo = i
					} else if i.Path == "/users/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c/Projects" {
						embeddedInfo = i
					}

				}
				Expect(fileInfo).ToNot(BeNil())
				Expect(fileInfo.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(fileInfo.Path).To(Equal("/users/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c/file.txt"))
				Expect(fileInfo.Size).To(Equal(uint64(1)))

				Expect(embeddedInfo).ToNot(BeNil())
				Expect(embeddedInfo.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(embeddedInfo.Path).To(Equal("/users/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c/Projects"))
				Expect(embeddedInfo.Size).To(Equal(uint64(2)))
			})

			It("lists the embedded project space", func() {
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
				Expect(info.Path).To(Equal(homeRef.Path))
				Expect(info.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(info.Size).To(Equal(uint64(3))) // home: 1, embedded: 2
			})

			It("stats the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: embeddedRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				info := statRes.Info
				Expect(info.Type).To(Equal(storagep.ResourceType_RESOURCE_TYPE_CONTAINER))
				Expect(info.Path).To(Equal(embeddedRef.Path))
				Expect(info.Owner.OpaqueId).To(Equal(user.Id.OpaqueId))
				Expect(info.Size).To(Equal(uint64(2)))
			})

			It("propagates Sizes from within the root space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(3)))

				By("Uploading a new file")
				err = fs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1234567890"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(13)))

				By("Uploading a new file into a subdir")
				err = fs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())
				err = fs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1234567890"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(23)))

				By("Updating an existing file")
				err = fs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("12345678901234567890"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(33)))
			})

			It("propagates Sizes from within the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(3)))

				By("Uploading a new file")
				err = embeddedFs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1234567890"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(13)))

				By("Uploading a new file into a subdir")
				err = embeddedFs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())
				err = embeddedFs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1234567890"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Size).To(Equal(uint64(23)))

				By("Updating an existing file")
				err = embeddedFs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("12345678901234567890"))))
				Expect(err).ToNot(HaveOccurred())

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
				err = fs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag := statRes.Info.Etag

				Expect(newEtag).ToNot(Equal(etag))

				By("Creating a new dir")
				err = fs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag2 := statRes.Info.Etag

				Expect(newEtag2).ToNot(Equal(newEtag))

				By("Updating an existing file")
				err = fs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: homeSpace.Id.OpaqueId}, Path: "/file.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("2"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag3 := statRes.Info.Etag

				Expect(newEtag3).ToNot(Equal(newEtag2))
			})

			It("propagates Etags from within the embedded space", func() {
				statRes, err := serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				etag := statRes.Info.Etag

				By("Uploading a new file")
				err = embeddedFs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag := statRes.Info.Etag

				Expect(newEtag).ToNot(Equal(etag))

				By("Creating a new dir")
				err = embeddedFs.CreateDir(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newdir"})
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag2 := statRes.Info.Etag

				Expect(newEtag2).ToNot(Equal(newEtag))

				By("Updating an existing file")
				err = embeddedFs.Upload(ctx, &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: embeddedSpace.Id.OpaqueId}, Path: "/newfile.txt"}, ioutil.NopCloser(bytes.NewReader([]byte("1"))))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(ctx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				newEtag3 := statRes.Info.Etag

				Expect(newEtag3).ToNot(Equal(newEtag2))
			})
		})
	})
})
