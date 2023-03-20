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

package blobstore_test

import (
	"io"
	"os"
	"path"

	"github.com/cs3org/reva/v2/pkg/storage/fs/ocis/blobstore"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/tests/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Blobstore", func() {
	var (
		tmpRoot     string
		blobNode    *node.Node
		blobPath    string
		blobSrcFile string
		data        []byte

		bs *blobstore.Blobstore
	)

	BeforeEach(func() {
		var err error
		tmpRoot, err = helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		data = []byte("1234567890")
		blobNode = &node.Node{
			SpaceID: "wonderfullspace",
			BlobID:  "huuuuugeblob",
		}
		blobPath = path.Join(tmpRoot, "spaces", "wo", "nderfullspace", "blobs", "hu", "uu", "uu", "ge", "blob")

		blobSrcFile = path.Join(tmpRoot, "blobsrc")

		bs, err = blobstore.New(path.Join(tmpRoot))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	It("creates the root directory if it doesn't exist", func() {
		_, err := os.Stat(path.Join(tmpRoot))
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Blob upload", func() {
		Describe("Upload", func() {
			BeforeEach(func() {
				Expect(os.WriteFile(blobSrcFile, data, 0700)).To(Succeed())
			})
			It("writes the blob", func() {
				err := bs.Upload(blobNode, blobSrcFile)
				Expect(err).ToNot(HaveOccurred())

				writtenBytes, err := os.ReadFile(blobPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(writtenBytes).To(Equal(data))
			})
		})
	})

	Context("with an existing blob", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(path.Dir(blobPath), 0700)).To(Succeed())
			Expect(os.WriteFile(blobPath, data, 0700)).To(Succeed())
		})

		Describe("Download", func() {
			It("cleans the key", func() {
				reader, err := bs.Download(blobNode)
				Expect(err).ToNot(HaveOccurred())

				readData, err := io.ReadAll(reader)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(data))
			})

			It("returns a reader to the blob", func() {
				reader, err := bs.Download(blobNode)
				Expect(err).ToNot(HaveOccurred())

				readData, err := io.ReadAll(reader)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(data))
			})
		})

		Describe("Delete", func() {
			It("deletes the blob", func() {
				_, err := os.Stat(blobPath)
				Expect(err).ToNot(HaveOccurred())

				err = bs.Delete(blobNode)
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(blobPath)
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
