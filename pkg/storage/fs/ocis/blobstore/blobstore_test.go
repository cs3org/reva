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
	"bytes"
	"io/ioutil"
	"os"
	"path"

	"github.com/cs3org/reva/pkg/storage/fs/ocis/blobstore"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Blobstore", func() {
	var (
		tmpRoot  string
		key      string
		blobPath string
		data     []byte

		bs *blobstore.Blobstore
	)

	BeforeEach(func() {
		var err error
		tmpRoot, err = helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		data = []byte("1234567890")
		key = "foo"
		blobPath = path.Join(tmpRoot, "blobs", key)

		bs, err = blobstore.New(path.Join(tmpRoot, "blobs"))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	It("creates the root directory if it doesn't exist", func() {
		_, err := os.Stat(path.Join(tmpRoot, "blobs"))
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Upload", func() {
		It("writes the blob", func() {
			err := bs.Upload(key, bytes.NewReader(data))
			Expect(err).ToNot(HaveOccurred())

			writtenBytes, err := ioutil.ReadFile(blobPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(writtenBytes).To(Equal(data))
		})
	})

	Context("with an existing blob", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(blobPath, data, 0700)).To(Succeed())
		})

		Describe("Download", func() {
			It("cleans the key", func() {
				reader, err := bs.Download("../" + key)
				Expect(err).ToNot(HaveOccurred())

				readData, err := ioutil.ReadAll(reader)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(data))
			})

			It("returns a reader to the blob", func() {
				reader, err := bs.Download(key)
				Expect(err).ToNot(HaveOccurred())

				readData, err := ioutil.ReadAll(reader)
				Expect(err).ToNot(HaveOccurred())
				Expect(readData).To(Equal(data))
			})
		})

		Describe("Delete", func() {
			It("deletes the blob", func() {
				_, err := os.Stat(blobPath)
				Expect(err).ToNot(HaveOccurred())

				err = bs.Delete(key)
				Expect(err).ToNot(HaveOccurred())

				_, err = os.Stat(blobPath)
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
