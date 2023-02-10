package xattrs_test

import (
	"os"
	"path"
	"strings"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Backend", func() {
	var (
		tmpdir string
		file   string
	)

	BeforeEach(func() {
		var err error
		tmpdir, err = os.MkdirTemp(os.TempDir(), "XattrsBackendTest-")
		Expect(err).ToNot(HaveOccurred())

		file = path.Join(tmpdir, "file")
		_, err = os.Create(file)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	})

	Describe("IniBackend", func() {
		var (
			backend = xattrs.IniBackend{}
		)

		Describe("Set", func() {
			It("sets an attribute", func() {
				err := backend.Set(file, "foo", "bar")
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("foo = bar\n"))
			})

			It("updates an attribute", func() {
				err := backend.Set(file, "foo", "bar")
				Expect(err).ToNot(HaveOccurred())
				err = backend.Set(file, "foo", "baz")
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(content)).To(Equal("foo = baz\n"))
			})
		})

		Describe("SetMultiple", func() {
			It("sets attributes", func() {
				err := backend.SetMultiple(file, map[string]string{"foo": "bar", "baz": "qux"})
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(file)
				Expect(err).ToNot(HaveOccurred())
				lines := strings.Split(strings.Trim(string(content), "\n"), "\n")
				Expect(lines).To(ConsistOf("foo = bar", "baz = qux"))
			})

			It("updates an attribute", func() {
				err := backend.Set(file, "foo", "bar")
				Expect(err).ToNot(HaveOccurred())
				err = backend.SetMultiple(file, map[string]string{"foo": "bar", "baz": "qux"})
				Expect(err).ToNot(HaveOccurred())

				content, err := os.ReadFile(file)
				Expect(err).ToNot(HaveOccurred())
				lines := strings.Split(strings.Trim(string(content), "\n"), "\n")
				Expect(lines).To(ConsistOf("foo = bar", "baz = qux"))
			})
		})

		Describe("All", func() {
			It("returns the entries", func() {
				err := os.WriteFile(file, []byte("foo=123\nbar=baz"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.All(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(v)).To(Equal(2))
				Expect(v["foo"]).To(Equal("123"))
				Expect(v["bar"]).To(Equal("baz"))
			})

			It("returns an empty map", func() {
				v, err := backend.All(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal(map[string]string{}))
			})
		})

		Describe("List", func() {
			It("returns the entries", func() {
				err := os.WriteFile(file, []byte("foo = 123\nbar = baz"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.List(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(ConsistOf("foo", "bar"))
			})

			It("returns an empty list", func() {
				v, err := backend.List(file)
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal([]string{}))
			})
		})

		Describe("Get", func() {
			It("returns the attribute", func() {
				err := os.WriteFile(file, []byte("foo = \"bar\"\n"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.Get(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal("bar"))
			})

			It("returns an error on unknown attributes", func() {
				_, err := backend.Get(file, "foo")
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("GetInt64", func() {
			It("returns the attribute", func() {
				err := os.WriteFile(file, []byte("foo=123\n"), 0600)
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
				err := os.WriteFile(file, []byte("foo=bar\n"), 0600)
				Expect(err).ToNot(HaveOccurred())

				v, err := backend.Get(file, "foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(v).To(Equal("bar"))

				err = backend.Remove(file, "foo")
				Expect(err).ToNot(HaveOccurred())

				_, err = backend.Get(file, "foo")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
