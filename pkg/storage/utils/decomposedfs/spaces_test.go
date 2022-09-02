// Copyright 2018-2022 CERN
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

package decomposedfs_test

import (
	"context"
	"fmt"
	"os"

	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

var _ = Describe("Spaces", func() {

	Describe("Create Space", func() {
		var (
			env *helpers.TestEnv
		)
		BeforeEach(func() {
			var err error
			env, err = helpers.NewTestEnv(nil)
			Expect(err).ToNot(HaveOccurred())
			env.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(
				func(ctx context.Context, in *cs3permissions.CheckPermissionRequest, opts ...grpc.CallOption) *cs3permissions.CheckPermissionResponse {
					if ctxpkg.ContextMustGetUser(ctx).Id.GetOpaqueId() == "25b69780-5f39-43be-a7ac-a9b9e9fe4230" {
						// id of owner/admin
						return &cs3permissions.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}
					}
					// id of generic user
					return &cs3permissions.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_PERMISSION_DENIED}}
				},
				nil)
			env.Permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(
				func(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) bool {
					return ctxpkg.ContextMustGetUser(ctx).Id.GetOpaqueId() == "25b69780-5f39-43be-a7ac-a9b9e9fe4230" // id of owner/admin
				},
				func(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) error {
					if ctxpkg.ContextMustGetUser(ctx).Id.GetOpaqueId() == "25b69780-5f39-43be-a7ac-a9b9e9fe4230" {
						// id of owner/admin
						return nil
					}
					// id of generic user
					return errtypes.PermissionDenied(fmt.Sprintf("user is not allowed to delete home space %s", n.ID))

				})
		})

		AfterEach(func() {
			if env != nil {
				env.Cleanup()
			}
		})

		Context("during login", func() {
			It("space is created", func() {
				resp, err := env.Fs.ListStorageSpaces(env.Ctx, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(resp)).To(Equal(1))
				Expect(string(resp[0].Opaque.GetMap()["spaceAlias"].Value)).To(Equal("personal/username"))
				Expect(resp[0].Name).To(Equal("username"))
				Expect(resp[0].SpaceType).To(Equal("personal"))
			})
		})
		Context("when creating a space", func() {
			It("project space is created", func() {
				env.Owner = nil
				resp, err := env.Fs.CreateStorageSpace(env.Ctx, &provider.CreateStorageSpaceRequest{Name: "Mission to Mars", Type: "project"})
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(resp.StorageSpace).ToNot(Equal(nil))
				Expect(string(resp.StorageSpace.Opaque.Map["spaceAlias"].Value)).To(Equal("project/mission-to-mars"))
				Expect(resp.StorageSpace.Name).To(Equal("Mission to Mars"))
				Expect(resp.StorageSpace.SpaceType).To(Equal("project"))
			})
		})

		Context("needs to check node permissions", func() {
			It("returns false on requesting for other user with canlistallspaces und no unrestricted privilege", func() {
				resp := env.Fs.MustCheckNodePermissions(env.Ctx, false)
				Expect(resp).To(Equal(true))
			})
			It("returns true on requesting unrestricted as non-admin", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Users[0])
				resp := env.Fs.MustCheckNodePermissions(ctx, true)
				Expect(resp).To(Equal(true))
			})
			It("returns true on requesting for own spaces", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Users[0])
				resp := env.Fs.MustCheckNodePermissions(ctx, false)
				Expect(resp).To(Equal(true))
			})
			It("returns false on unrestricted", func() {
				resp := env.Fs.MustCheckNodePermissions(env.Ctx, true)
				Expect(resp).To(Equal(false))
			})
		})

		Context("can list spaces of requested user", func() {
			It("returns false on requesting for other user as non-admin", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Users[0])
				res := env.Fs.CanListSpacesOfRequestedUser(ctx, helpers.User1ID)
				Expect(res).To(Equal(false))
			})
			It("returns true on requesting for other user as admin", func() {
				res := env.Fs.CanListSpacesOfRequestedUser(env.Ctx, helpers.User0ID)
				Expect(res).To(Equal(true))
			})
			It("returns true on requesting for own spaces", func() {
				res := env.Fs.CanListSpacesOfRequestedUser(env.Ctx, helpers.OwnerID)
				Expect(res).To(Equal(true))
			})
		})

		Context("can delete homespace", func() {
			It("fails on trying to delete a homespace as non-admin", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Users[1])
				resp, err := env.Fs.ListStorageSpaces(env.Ctx, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(resp)).To(Equal(1))
				Expect(string(resp[0].Opaque.GetMap()["spaceAlias"].Value)).To(Equal("personal/username"))
				Expect(resp[0].Name).To(Equal("username"))
				Expect(resp[0].SpaceType).To(Equal("personal"))
				err = env.Fs.DeleteStorageSpace(ctx, &provider.DeleteStorageSpaceRequest{
					Id: resp[0].GetId(),
				})
				Expect(err).To(HaveOccurred())
			})
			It("succeeds on trying to delete homespace as admin", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Owner)
				resp, err := env.Fs.ListStorageSpaces(env.Ctx, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(resp)).To(Equal(1))
				Expect(string(resp[0].Opaque.GetMap()["spaceAlias"].Value)).To(Equal("personal/username"))
				Expect(resp[0].Name).To(Equal("username"))
				Expect(resp[0].SpaceType).To(Equal("personal"))
				err = env.Fs.DeleteStorageSpace(ctx, &provider.DeleteStorageSpaceRequest{
					Id: resp[0].GetId(),
				})
				Expect(err).To(Not(HaveOccurred()))
			})
		})

		Describe("Create Spaces with custom alias template", func() {
			var (
				env *helpers.TestEnv
			)

			BeforeEach(func() {
				var err error
				env, err = helpers.NewTestEnv(map[string]interface{}{
					"personalspacealias_template": "{{.SpaceType}}/{{.Email.Local}}@{{.Email.Domain}}",
					"generalspacealias_template":  "{{.SpaceType}}:{{.SpaceName | replace \" \" \"-\" | upper}}",
				})
				Expect(err).ToNot(HaveOccurred())
				env.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&cs3permissions.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil)
			})

			AfterEach(func() {
				if env != nil {
					env.Cleanup()
				}
			})
			Context("during login", func() {
				It("personal space is created with custom alias", func() {
					resp, err := env.Fs.ListStorageSpaces(env.Ctx, nil, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(resp)).To(Equal(1))
					Expect(string(resp[0].Opaque.GetMap()["spaceAlias"].Value)).To(Equal("personal/username@_unknown"))
				})
			})
			Context("creating a space", func() {
				It("project space is created with custom alias", func() {
					env.Owner = nil
					resp, err := env.Fs.CreateStorageSpace(env.Ctx, &provider.CreateStorageSpaceRequest{Name: "Mission to Venus", Type: "project"})
					Expect(err).ToNot(HaveOccurred())
					Expect(resp.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
					Expect(resp.StorageSpace).ToNot(Equal(nil))
					Expect(string(resp.StorageSpace.Opaque.Map["spaceAlias"].Value)).To(Equal("project:MISSION-TO-VENUS"))

				})
			})
		})
	})

	Describe("ReadSpaceAndNodeFromSpaceTypeLink", func() {
		var (
			tmpdir string
		)

		BeforeEach(func() {
			tmpdir, _ = os.MkdirTemp(os.TempDir(), "ReadSpaceAndNodeFromSpaceTypeLink-")
		})

		AfterEach(func() {
			if tmpdir != "" {
				os.RemoveAll(tmpdir)
			}
		})

		DescribeTable("ReadSpaceAndNodeFromSpaceTypeLink",
			func(link string, expectSpace string, expectedNode string, shouldErr bool) {
				space, node, err := decomposedfs.ReadSpaceAndNodeFromIndexLink(link)
				if shouldErr {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(space).To(Equal(expectSpace))
				Expect(node).To(Equal(expectedNode))
			},

			Entry("invalid number of slashes", "../../../spaces/sp_ace-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("does not contain spaces", "../../../spac_s/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("does not contain nodes", "../../../spaces/sp/ace-id/nod_s/sh/or/tn/od/eid", "", "", true),
			Entry("does not start with ..", "_./../../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("does not start with ../..", "../_./../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("does not start with ../../..", "../_./../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("invalid", "../../../spaces/space-id/nodes/sh/or/tn/od/eid", "", "", true),
			Entry("uuid", "../../../spaces/4c/510ada-c86b-4815-8820-42cdf82c3d51/nodes/4c/51/0a/da/-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51", false),
			Entry("uuid", "../../../spaces/4c/510ada-c86b-4815-8820-42cdf82c3d51/nodes/4c/51/0a/da/-c86b-4815-8820-42cdf82c3d51.T.2022-02-24T12:35:18.196484592Z", "4c510ada-c86b-4815-8820-42cdf82c3d51", "4c510ada-c86b-4815-8820-42cdf82c3d51.T.2022-02-24T12:35:18.196484592Z", false),
			Entry("short", "../../../spaces/sp/ace-id/nodes/sh/or/tn/od/eid", "space-id", "shortnodeid", false),
		)
	})

	Describe("Update Space", func() {
		var (
			env *helpers.TestEnv
		)
		BeforeEach(func() {
			var err error
			env, err = helpers.NewTestEnv(nil)
			Expect(err).ToNot(HaveOccurred())
			env.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(
				func(ctx context.Context, in *cs3permissions.CheckPermissionRequest, opts ...grpc.CallOption) *cs3permissions.CheckPermissionResponse {
					if ctxpkg.ContextMustGetUser(ctx).Id.GetOpaqueId() == "25b69780-5f39-43be-a7ac-a9b9e9fe4230" {
						// id of owner/admin
						return &cs3permissions.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}
					}
					// id of generic user
					return &cs3permissions.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_PERMISSION_DENIED}}
				},
				nil)

		})

		AfterEach(func() {
			if env != nil {
				env.Cleanup()
			}
		})
		Context("project space", func() {
			It("Create a project space as admin and change quota", func() {
				resp, err := env.Fs.CreateStorageSpace(env.Ctx, &provider.CreateStorageSpaceRequest{Name: "Mission to Venus", Type: "project"})
				Expect(err).ToNot(HaveOccurred())
				updateResp, err := env.Fs.UpdateStorageSpace(
					env.Ctx,
					&provider.UpdateStorageSpaceRequest{
						StorageSpace: &provider.StorageSpace{
							Id:    resp.StorageSpace.Id,
							Quota: &provider.Quota{QuotaMaxBytes: uint64(1000)},
						},
					},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(updateResp.Status.Code, rpcv1beta1.Code_CODE_OK)
				Expect(updateResp.StorageSpace.Quota.QuotaMaxBytes, uint64(1000))
			})
			It("try to change quota as a non admin user", func() {
				ctx := ctxpkg.ContextSetUser(context.Background(), env.Users[0])
				updateResp, err := env.Fs.UpdateStorageSpace(
					ctx,
					&provider.UpdateStorageSpaceRequest{
						StorageSpace: &provider.StorageSpace{
							Id: &provider.StorageSpaceId{
								OpaqueId: storagespace.FormatResourceID(*env.SpaceRootRes),
							},
							Quota: &provider.Quota{QuotaMaxBytes: uint64(1000)},
						},
					},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(updateResp.Status.Code, rpcv1beta1.Code_CODE_PERMISSION_DENIED)
			})
		})
	})
})
