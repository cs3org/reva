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

package decomposedfs_test

import (
	"bytes"
	"errors"
	"io"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/node"
	helpers "github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("CommitUpload", func() {
	var (
		env *helpers.TestEnv
		ref *provider.Reference
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		ref = &provider.Reference{
			ResourceId: env.SpaceRootRes,
			Path:       "/dir1/new-file.txt",
		}

		// TouchFile-first protocol: node must exist before CommitUpload is called.
		env.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).
			Return(&provider.ResourcePermissions{InitiateFileUpload: true, Stat: true}, nil).Times(1)
		_, err = env.Fs.TouchFile(env.Ctx, ref, false, "")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	makeSource := func(content []byte) storage.UploadSource {
		return storage.UploadSource{
			Body:   io.NopCloser(bytes.NewReader(content)),
			Length: int64(len(content)),
		}
	}

	Context("when sessionID is empty", func() {
		It("fails with BadRequest", func() {
			err := env.Fs.CommitUpload(env.Ctx, ref, "", makeSource([]byte("x")))
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsBadRequest)
			Expect(ok).To(BeTrue(), "expected errtypes.BadRequest, got %T: %v", err, err)
		})
	})

	Context("when node does not exist", func() {
		It("fails with NotFound", func() {
			missingRef := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/never-created.txt",
			}
			err := env.Fs.CommitUpload(env.Ctx, missingRef, "session-1", makeSource([]byte("x")))
			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsNotFound)
			Expect(ok).To(BeTrue(), "expected errtypes.NotFound, got %T: %v", err, err)
		})
	})

	Context("on a new file", func() {
		It("writes bytes to the blobstore", func() {
			env.Blobstore.On("UploadFromReader", mock.AnythingOfType("*node.Node"), mock.Anything, mock.AnythingOfType("int64")).Return(nil)

			err := env.Fs.CommitUpload(env.Ctx, ref, "session-1", makeSource([]byte("hello reva")))

			Expect(err).ToNot(HaveOccurred())
			env.Blobstore.AssertCalled(GinkgoT(), "UploadFromReader", mock.AnythingOfType("*node.Node"), mock.Anything, mock.AnythingOfType("int64"))
		})
	})

	Context("on overwrite", func() {
		It("writes to a different blob slot on each commit", func() {
			var capturedNodes []*node.Node
			env.Blobstore.On("UploadFromReader", mock.AnythingOfType("*node.Node"), mock.Anything, mock.AnythingOfType("int64")).
				Run(func(args mock.Arguments) {
					n := args.Get(0).(*node.Node)
					capturedNodes = append(capturedNodes, n)
				}).Return(nil)

			err := env.Fs.CommitUpload(env.Ctx, ref, "session-1", makeSource([]byte("original content")))
			Expect(err).ToNot(HaveOccurred())

			err = env.Fs.CommitUpload(env.Ctx, ref, "session-2", makeSource([]byte("brand new content")))
			Expect(err).ToNot(HaveOccurred())

			env.Blobstore.AssertNumberOfCalls(GinkgoT(), "UploadFromReader", 2)
			Expect(capturedNodes).To(HaveLen(2))
			Expect(capturedNodes[0].BlobID).To(Equal("session-1"))
			Expect(capturedNodes[1].BlobID).To(Equal("session-2"))
		})
	})

	Context("after a successful commit", func() {
		It("propagates etag change to the parent directory", func() {
			env.Blobstore.On("UploadFromReader", mock.AnythingOfType("*node.Node"), mock.Anything, mock.AnythingOfType("int64")).Return(nil)

			parentRef := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1",
			}
			parentNode, err := env.Lookup.NodeFromResource(env.Ctx, parentRef)
			Expect(err).ToNot(HaveOccurred())
			tmBefore, _ := parentNode.GetTMTime(env.Ctx)

			err = env.Fs.CommitUpload(env.Ctx, ref, "session-1", makeSource([]byte("hello")))
			Expect(err).ToNot(HaveOccurred())

			parentNode, err = env.Lookup.NodeFromResource(env.Ctx, parentRef)
			Expect(err).ToNot(HaveOccurred())
			tmAfter, err := parentNode.GetTMTime(env.Ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(tmAfter).To(BeTemporally(">=", tmBefore))
		})
	})

	Context("when WriteBlob fails", func() {
		It("cleans up the orphaned blob and returns an error", func() {
			writeErr := errors.New("blobstore unavailable")
			env.Blobstore.On("UploadFromReader", mock.AnythingOfType("*node.Node"), mock.Anything, mock.AnythingOfType("int64")).Return(writeErr)
			env.Blobstore.On("Delete", mock.AnythingOfType("*node.Node")).Return(nil)

			err := env.Fs.CommitUpload(env.Ctx, ref, "session-1", makeSource([]byte("hello")))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("blobstore unavailable"))
			env.Blobstore.AssertCalled(GinkgoT(), "Delete", mock.AnythingOfType("*node.Node"))
		})
	})
})
