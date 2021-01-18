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

package s3ng_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/mock"

	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/mocks"
	ruser "github.com/cs3org/reva/pkg/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("File uploads", func() {
	var (
		ref  *provider.Reference
		fs   storage.FS
		user *userpb.User
		ctx  context.Context

		options     map[string]interface{}
		lookup      *s3ng.Lookup
		permissions *mocks.PermissionsChecker
	)

	BeforeEach(func() {
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: "/foo",
			},
		}
		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "userid",
			},
			Username: "username",
		}
		ctx = ruser.ContextSetUser(context.Background(), user)

		tmpRoot, err := ioutil.TempDir("", "reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"Root":          tmpRoot,
			"s3.endpoint":   "http://1.2.3.4:5000",
			"s3.region":     "default",
			"s3.bucket":     "the-bucket",
			"s3.access_key": "foo",
			"s3.secret_key": "bar",
		}
		lookup = &s3ng.Lookup{}
		permissions = &mocks.PermissionsChecker{}
	})

	AfterEach(func() {
		root := options["Root"].(string)
		if strings.HasPrefix(root, os.TempDir()) {
			os.RemoveAll(root)
		}
	})

	JustBeforeEach(func() {
		var err error
		fs, err = s3ng.New(options, lookup, permissions)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with insufficient permissions", func() {
		BeforeEach(func() {
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		})

		Describe("InitiateUpload", func() {
			It("fails", func() {
				_, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})
				Expect(err).To(MatchError("error: permission denied: root/foo"))
			})
		})
	})

	Context("with sufficient permissions", func() {
		BeforeEach(func() {
			permissions.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
		})

		Describe("InitiateUpload", func() {
			It("returns uploadIds for simple and tus uploads", func() {
				uploadIds, err := fs.InitiateUpload(ctx, ref, 10, map[string]string{})

				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadIds)).To(Equal(2))
				Expect(uploadIds["simple"]).ToNot(BeEmpty())
				Expect(uploadIds["tus"]).ToNot(BeEmpty())
			})
		})

		Describe("Upload", func() {
			It("stores the blob in s3", func() {
				data := []byte("0123456789")

				err := fs.Upload(ctx, ref, ioutil.NopCloser(bytes.NewReader(data)))
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
