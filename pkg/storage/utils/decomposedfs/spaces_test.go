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
	permissionsv1beta1 "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	helpers "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Create Spaces", func() {
	var (
		env *helpers.TestEnv
	)

	BeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())
		env.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&permissionsv1beta1.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil)
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
			resp, err := env.Fs.CreateStorageSpace(env.Ctx, &provider.CreateStorageSpaceRequest{Name: "Mission to Mars", Type: "project"})
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			Expect(resp.StorageSpace).ToNot(Equal(nil))
			Expect(string(resp.StorageSpace.Opaque.Map["spaceAlias"].Value)).To(Equal("project/mission-to-mars"))
			Expect(resp.StorageSpace.Name).To(Equal("Mission to Mars"))
			Expect(resp.StorageSpace.SpaceType).To(Equal("project"))
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
			env.PermissionsClient.On("CheckPermission", mock.Anything, mock.Anything, mock.Anything).Return(&permissionsv1beta1.CheckPermissionResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil)
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
				resp, err := env.Fs.CreateStorageSpace(env.Ctx, &provider.CreateStorageSpaceRequest{Name: "Mission to Venus", Type: "project"})
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				Expect(resp.StorageSpace).ToNot(Equal(nil))
				Expect(string(resp.StorageSpace.Opaque.Map["spaceAlias"].Value)).To(Equal("project:MISSION-TO-VENUS"))

			})
		})
	})
})
