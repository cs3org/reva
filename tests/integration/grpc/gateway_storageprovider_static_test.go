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
	"github.com/cs3org/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	jwt "github.com/cs3org/reva/v2/pkg/token/manager/jwt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This test suite tests the gprc gateway interface
//
// It uses the `startRevads` helper to spawn the according reva daemon and
// other dependencies like a userprovider if needed.
// It also sets up an authenticated context and a service client to the storage
// provider to be used in the assertion functions.
var _ = PDescribe("gateway using a static registry and a shard setup", func() {
	// TODO: Static registry relies on gateway being not dumb  at the moment. So these won't work anymore
	// FIXME: Bring me back please!
	var (
		dependencies = map[string]string{}
		revads       = map[string]*Revad{}

		einsteinCtx   context.Context
		marieCtx      context.Context
		variables     map[string]string
		serviceClient gateway.GatewayAPIClient
		marie         = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:39000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "marie",
		}
		einstein = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:39000",
				OpaqueId: "e4fb0282-fabf-4cff-b1ee-90bdc01c4eef",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}
		homeRef = &storagep.Reference{Path: "."}
	)

	BeforeEach(func() {
		dependencies = map[string]string{
			"gateway":  "gateway-static.toml",
			"users":    "userprovider-json.toml",
			"storage":  "storageprovider-local.toml",
			"storage2": "storageprovider-local.toml",
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
		einsteinCtx = context.Background()
		marieCtx = context.Background()

		// Add auth token
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		scope, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())

		t, err := tokenManager.MintToken(marieCtx, marie, scope)
		Expect(err).ToNot(HaveOccurred())
		marieCtx = ctxpkg.ContextSetToken(marieCtx, t)
		marieCtx = metadata.AppendToOutgoingContext(marieCtx, ctxpkg.TokenHeader, t)
		marieCtx = ctxpkg.ContextSetUser(marieCtx, marie)

		t, err = tokenManager.MintToken(einsteinCtx, einstein, scope)
		Expect(err).ToNot(HaveOccurred())
		einsteinCtx = ctxpkg.ContextSetToken(einsteinCtx, t)
		einsteinCtx = metadata.AppendToOutgoingContext(einsteinCtx, ctxpkg.TokenHeader, t)
		einsteinCtx = ctxpkg.ContextSetUser(einsteinCtx, einstein)

		revads, err = startRevads(dependencies, variables)
		Expect(err).ToNot(HaveOccurred())
		Expect(revads["gateway"]).ToNot(BeNil())
		selector, err := pool.GatewaySelector(revads["gateway"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
		serviceClient, err = selector.Next()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
	})

	Context("with a mapping based home jail", func() {
		BeforeEach(func() {
			variables["disable_home"] = "false"
		})

		It("creates a home directory on the correct provider", func() {
			By("creating marie's home")
			statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err := serviceClient.CreateHome(marieCtx, &storagep.CreateHomeRequest{})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			// the mapping considers the opaque id: f... -> storage2
			fi, err := os.Stat(path.Join(revads["storage2"].StorageRoot, "data/f", marie.Id.OpaqueId))
			Expect(err).ToNot(HaveOccurred())
			Expect(fi.IsDir()).To(BeTrue())
			_, err = os.Stat(path.Join(revads["storage"].StorageRoot, "data/f", marie.Id.OpaqueId))
			Expect(err).To(HaveOccurred())

			ghRes, err := serviceClient.GetHome(marieCtx, &storagep.GetHomeRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ghRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			By("creating einstein's home")
			statRes, err = serviceClient.Stat(einsteinCtx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

			res, err = serviceClient.CreateHome(einsteinCtx, &storagep.CreateHomeRequest{})
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(err).ToNot(HaveOccurred())

			statRes, err = serviceClient.Stat(einsteinCtx, &storagep.StatRequest{Ref: homeRef})
			Expect(err).ToNot(HaveOccurred())
			Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			// the mapping considers the opaque id: e... -> storage
			fi, err = os.Stat(path.Join(revads["storage"].StorageRoot, "data/e", einstein.Id.OpaqueId))
			Expect(err).ToNot(HaveOccurred())
			Expect(fi.IsDir()).To(BeTrue())
			_, err = os.Stat(path.Join(revads["storage2"].StorageRoot, "data/e", einstein.Id.OpaqueId))
			Expect(err).To(HaveOccurred())

			ghRes, err = serviceClient.GetHome(einsteinCtx, &storagep.GetHomeRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ghRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
		})

		Context("with a home directory", func() {
			JustBeforeEach(func() {
				res, err := serviceClient.CreateHome(marieCtx, &storagep.CreateHomeRequest{})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})

			It("creates and lists a new directory", func() {
				newRef := &storagep.Reference{Path: "/home/newdir"}

				listRes, err := serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(1))
				Expect(listRes.Infos[0].Path).To(Equal("/home/MyShares"))

				statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: newRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))

				res, err := serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{Ref: newRef})
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: newRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				listRes, err = serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(listRes.Infos)).To(Equal(2))
				paths := []string{}
				for _, i := range listRes.Infos {
					paths = append(paths, i.Path)
				}
				Expect(paths).To(ConsistOf("/home/MyShares", newRef.Path))

				listRes, err = serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: newRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(listRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})

			Context("and a subdirectory", func() {
				var (
					subdirRef = &storagep.Reference{Path: "/home/subdir"}
				)

				JustBeforeEach(func() {
					createRes, err := serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{Ref: subdirRef})
					Expect(createRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
					Expect(err).ToNot(HaveOccurred())
				})

				It("gets the path to an ID", func() {
					statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: subdirRef})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					getPathRes, err := serviceClient.GetPath(marieCtx, &storagep.GetPathRequest{ResourceId: statRes.Info.Id})
					Expect(err).ToNot(HaveOccurred())
					Expect(getPathRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				})

				It("stats by path and by ID", func() {
					statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: subdirRef})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					idRef := &storagep.Reference{ResourceId: &storagep.ResourceId{StorageId: revads["storage2"].ID, OpaqueId: statRes.Info.Id.OpaqueId}}
					statRes, err = serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: idRef})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				})

				It("moves and deletes a directory", func() {
					newRef2 := &storagep.Reference{Path: "/home/newdir2"}

					moveRes, err := serviceClient.Move(marieCtx, &storagep.MoveRequest{Source: subdirRef, Destination: newRef2})
					Expect(err).ToNot(HaveOccurred())
					Expect(moveRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: newRef2})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					deleteRes, err := serviceClient.Delete(marieCtx, &storagep.DeleteRequest{Ref: newRef2})
					Expect(deleteRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
					Expect(err).ToNot(HaveOccurred())

					statRes, err = serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: newRef2})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
				})
			})
		})
	})

	Context("with a sharded /users mount", func() {
		var (
			homePath  = "/users/f/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"
			rootRef   = &storagep.Reference{Path: path.Join("/users")}
			baseRef   = &storagep.Reference{Path: path.Join("/users/f")}
			homeRef   = &storagep.Reference{Path: homePath}
			subdirRef = &storagep.Reference{Path: path.Join(homePath, "subdir")}
		)

		BeforeEach(func() {
			variables["disable_home"] = "true"
		})

		It("merges the results of both /users providers", func() {
			lRes, err := serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/users"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(lRes.Infos)).To(Equal(36))

			lRes, err = serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/users/f"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(lRes.Infos)).To(Equal(0))

			res, err := serviceClient.CreateContainer(einsteinCtx, &storagep.CreateContainerRequest{
				Ref: &storagep.Reference{
					Path: path.Join("/users/e"),
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			res, err = serviceClient.CreateContainer(einsteinCtx, &storagep.CreateContainerRequest{
				Ref: &storagep.Reference{
					Path: path.Join("/users/e", einstein.Id.OpaqueId),
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			lRes, err = serviceClient.ListContainer(einsteinCtx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/users/e"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(lRes.Infos)).To(Equal(1))
			Expect(lRes.Infos[0].Path).To(Equal("/users/e/e4fb0282-fabf-4cff-b1ee-90bdc01c4eef"))

			lRes, err = serviceClient.ListContainer(einsteinCtx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/users/d"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(lRes.Infos)).To(Equal(0))

			res, err = serviceClient.CreateContainer(einsteinCtx, &storagep.CreateContainerRequest{
				Ref: &storagep.Reference{
					Path: path.Join("/users/f"),
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			res, err = serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{
				Ref: &storagep.Reference{
					Path: path.Join("/users/f", marie.Id.OpaqueId),
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			lRes, err = serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: &storagep.Reference{Path: "/users/f"}})
			Expect(err).ToNot(HaveOccurred())
			Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(len(lRes.Infos)).To(Equal(1))
			Expect(lRes.Infos[0].Path).To(Equal("/users/f/f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"))
		})

		Context("with a user home", func() {
			JustBeforeEach(func() {
				res, err := serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{
					Ref: &storagep.Reference{
						Path: path.Join("/users/f"),
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				res, err = serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{
					Ref: &storagep.Reference{
						Path: path.Join("/users/f", marie.Id.OpaqueId),
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			})

			It("provides access to the user home", func() {
				newRef := &storagep.Reference{Path: path.Join(homePath, "newName")}

				createRes, err := serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{Ref: subdirRef})
				Expect(createRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(err).ToNot(HaveOccurred())

				statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(statRes.Info.Path).To(Equal(homePath))

				lRes, err := serviceClient.ListContainer(marieCtx, &storagep.ListContainerRequest{Ref: homeRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(lRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(len(lRes.Infos)).To(Equal(1))
				Expect(lRes.Infos[0].Path).To(Equal(subdirRef.Path))

				mRes, err := serviceClient.Move(marieCtx, &storagep.MoveRequest{Source: subdirRef, Destination: newRef})
				Expect(mRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(err).ToNot(HaveOccurred())

				dRes, err := serviceClient.Delete(marieCtx, &storagep.DeleteRequest{Ref: newRef})
				Expect(dRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(err).ToNot(HaveOccurred())

				statRes, err = serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: newRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_NOT_FOUND))
			})

			It("propagates the etag to the root", func() {
				getEtag := func(r *storagep.Reference) string {
					statRes, err := serviceClient.Stat(marieCtx, &storagep.StatRequest{Ref: r})
					Expect(err).ToNot(HaveOccurred())
					Expect(statRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
					return statRes.Info.Etag
				}

				rootEtag := getEtag(rootRef)
				baseEtag := getEtag(baseRef)
				userEtag := getEtag(homeRef)

				createRes, err := serviceClient.CreateContainer(marieCtx, &storagep.CreateContainerRequest{Ref: subdirRef})
				Expect(err).ToNot(HaveOccurred())
				Expect(createRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				Expect(getEtag(homeRef)).ToNot(Equal(userEtag))
				Expect(getEtag(baseRef)).ToNot(Equal(baseEtag))
				Expect(getEtag(rootRef)).ToNot(Equal(rootEtag))
			})
		})
	})
})
