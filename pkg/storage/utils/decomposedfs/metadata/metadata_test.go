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

package metadata_test

import (
	"os"
	"path"
	"strings"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Backend", func() {
	var (
		tmpdir   string
		file     string
		metafile string

		backend metadata.Backend
	)

	BeforeEach(func() {
		var err error
		tmpdir, err = os.MkdirTemp(os.TempDir(), "XattrsBackendTest-")
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		file = path.Join(tmpdir, "file")
		metafile = backend.MetadataPath(file)
		_, err := os.Create(metafile)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("IniBackend", func() {
		BeforeEach(func() {
			backend = metadata.NewIniBackend(tmpdir, options.CacheOptions{})
		})

		Describe("Set", func() {
			It("sets an attribute", func() {
				err := backend.Set(file, "foo", []byte("bar"))
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("foo = bar\n"))
			})

			It("updates an attribute", func() {
				err := backend.Set(file, "foo", []byte("bar"))
				Expect(err).ToNot(HaveOccurred())
				err = backend.Set(file, "foo", []byte("baz"))
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("foo = baz\n"))
			})

			It("encodes where needed", func() {
				err := backend.Set(file, "user.ocis.cs.foo", []byte("bar"))
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("user.ocis.cs.foo = bar\n"))

				err = backend.Set(file, "user.ocis.cs.foo", []byte{200, 201, 202})
				Expect(err).ToNot(HaveOccurred())

				content, err = os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("`base64:user.ocis.cs.foo` = yMnK\n"))
			})

			It("doesn't encode already encoded attributes", func() {
				err := backend.Set(file, "user.ocis.cs.foo", []byte{200, 201, 202})
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("`base64:user.ocis.cs.foo` = yMnK\n"))

				err = backend.Set(file, "user.something", []byte("doesn'tmatter"))
				Expect(err).ToNot(HaveOccurred())

				content, err = os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("`base64:user.ocis.cs.foo` = yMnK\n"))
			})

			It("sets an empty attribute", func() {
				_, err := backend.Get(file, "foo")
				Expect(err).To(HaveOccurred())

				err = backend.Set(file, "foo", []byte{})
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.Get(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal(""))
			})
		})

		Describe("SetMultiple", func() {
			It("sets attributes", func() {
				err := backend.SetMultiple(file, map[string][]byte{"foo": []byte("bar"), "baz": []byte("qux")}, true)
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				lines := strings.Split(strings.Trim(string(content), "\n"), "\n")
				Expect(lines).To(ConsistOf("foo = bar", "baz = qux"))
			})

			It("updates an attribute", func() {
				err := backend.Set(file, "foo", []byte("bar"))
				Expect(err).ToNot(HaveOccurred())
				err = backend.SetMultiple(file, map[string][]byte{"foo": []byte("bar"), "baz": []byte("qux")}, true)
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				lines := strings.Split(strings.Trim(string(content), "\n"), "\n")
				Expect(lines).To(ConsistOf("foo = bar", "baz = qux"))
			})

			It("encodes where needed", func() {
				err := backend.SetMultiple(file, map[string][]byte{
					"user.ocis.something.foo": []byte("bar"),
					"user.ocis.cs.foo":        []byte{200, 201, 202},
					"user.ocis.md.foo":        []byte{200, 201, 202},
					"user.ocis.grant.foo":     []byte("bar"),
				}, true)
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(metafile)
				Expect(err).ToNot(HaveOccurred())
				expected := []string{
					"user.ocis.something.foo=bar",
					"`base64:user.ocis.cs.foo`=yMnK",
					"`base64:user.ocis.md.foo`=yMnK",
					"user.ocis.grant.foo=bar"}
				Expect(strings.Split(strings.ReplaceAll(strings.Trim(string(content), "\n"), " ", ""), "\n")).To(ConsistOf(expected))
			})
		})

		Describe("All", func() {
			It("returns the entries", func() {
				err := os.WriteFile(metafile, []byte("foo=123\nbar=baz"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.All(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(v)).To(Equal(2))
				Expect(v["foo"]).To(Equal("123"))
				Expect(v["bar"]).To(Equal([]byte("baz")))
			})

			It("returns an empty map", func() {
				v, err := backend.All(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal(map[string]string{}))
			})
		})

		Describe("List", func() {
			It("returns the entries", func() {
				err := os.WriteFile(metafile, []byte("foo = 123\nbar = baz"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.List(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(ConsistOf("foo", []byte("bar")))
			})

			It("returns an empty list", func() {
				v, err := backend.List(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal([]string{}))
			})
		})

		Describe("Get", func() {
			It("returns the attribute", func() {
				err := os.WriteFile(metafile, []byte("foo = \"bar\"\n"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.Get(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal([]byte("bar")))
			})

			It("returns an error on unknown attributes", func() {
				_, err := backend.Get(file, "foo")
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("GetInt64", func() {
			It("returns the attribute", func() {
				err := os.WriteFile(metafile, []byte("foo=123\n"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.GetInt64(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal(int64(123)))
			})

			It("returns an error on unknown attributes", func() {
				_, err := backend.GetInt64(file, "foo")
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("Get", func() {
			It("deletes an attribute", func() {
				err := os.WriteFile(metafile, []byte("foo=bar\n"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.Get(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal([]byte("bar")))

				err = backend.Remove(file, "foo")
				Expect(err).ToNot(HaveOccurred())

				_, err = backend.Get(file, "foo")
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("IsMetaFile", func() {
			It("returns true", func() {
				Expect(backend.IsMetaFile("foo.ini")).To(BeTrue())
			})

			It("returns false", func() {
				Expect(backend.IsMetaFile("foo.txt")).To(BeFalse())
			})
		})
	})
})
