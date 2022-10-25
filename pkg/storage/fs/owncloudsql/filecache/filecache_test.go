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

package filecache_test

import (
	"context"
	"database/sql"
	"os"
	"strconv"

	_ "github.com/mattn/go-sqlite3"

	"github.com/cs3org/reva/v2/pkg/storage/fs/owncloudsql/filecache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filecache", func() {
	var (
		cache      *filecache.Cache
		testDbFile *os.File
		sqldb      *sql.DB
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		testDbFile, err = os.CreateTemp("", "example")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := os.ReadFile("test.db")
		Expect(err).ToNot(HaveOccurred())

		_, err = testDbFile.Write(dbData)
		Expect(err).ToNot(HaveOccurred())
		err = testDbFile.Close()
		Expect(err).ToNot(HaveOccurred())

		sqldb, err = sql.Open("sqlite3", testDbFile.Name())
		Expect(err).ToNot(HaveOccurred())

		cache, err = filecache.New("sqlite3", sqldb)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(testDbFile.Name())
	})

	Describe("ListStorages", func() {
		It("returns all storages", func() {
			storages, err := cache.ListStorages(ctx, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(storages)).To(Equal(2))
			ids := []string{}
			numericIDs := []int{}
			for _, s := range storages {
				ids = append(ids, s.ID)
				numericIDs = append(numericIDs, s.NumericID)
			}
			Expect(numericIDs).To(ConsistOf([]int{1, 2}))
			Expect(ids).To(ConsistOf([]string{"home::admin", "local::/mnt/data/files/"}))
		})

		It("returns all home storages", func() {
			storages, err := cache.ListStorages(ctx, true)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(storages)).To(Equal(1))
			Expect(storages[0].ID).To(Equal("home::admin"))
			Expect(storages[0].NumericID).To(Equal(1))
		})
	})

	Describe("GetStorage", func() {
		It("returns an error when the id is invalid", func() {
			s, err := cache.GetStorage(ctx, "foo")
			Expect(err).To(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("returns an error when the id doesn't exist", func() {
			s, err := cache.GetStorage(ctx, 100)
			Expect(err).To(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("returns the storage", func() {
			s, err := cache.GetStorage(ctx, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(s.ID).To(Equal("home::admin"))
			Expect(s.NumericID).To(Equal(1))
		})

		It("takes string ids", func() {
			s, err := cache.GetStorage(ctx, "1")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.ID).To(Equal("home::admin"))
			Expect(s.NumericID).To(Equal(1))
		})
	})

	Describe("GetNumericStorageID", func() {
		It("returns the proper storage id", func() {
			nid, err := cache.GetNumericStorageID(ctx, "home::admin")
			Expect(err).ToNot(HaveOccurred())
			Expect(nid).To(Equal(1))
		})
	})

	Describe("GetStorageOwner", func() {
		It("returns the owner", func() {
			owner, err := cache.GetStorageOwner(ctx, "1")
			Expect(err).ToNot(HaveOccurred())
			Expect(owner).To(Equal("admin"))
		})
	})

	Describe("CreateStorage", func() {
		It("creates the storage and a root item", func() {
			id, err := cache.CreateStorage(ctx, "bar")
			Expect(err).ToNot(HaveOccurred())
			Expect(id > 0).To(BeTrue())

			owner, err := cache.GetStorageOwner(ctx, id)
			Expect(err).ToNot(HaveOccurred())
			Expect(owner).To(Equal("bar"))

			file, err := cache.Get(ctx, 1, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(file).ToNot(BeNil())
		})
	})
	Describe("GetStorageOwnerByFileID", func() {
		It("returns the owner", func() {
			owner, err := cache.GetStorageOwnerByFileID(ctx, "10")
			Expect(err).ToNot(HaveOccurred())
			Expect(owner).To(Equal("admin"))
		})
	})

	Describe("Get", func() {
		It("gets existing files", func() {
			path := "files/Photos/Portugal.jpg"
			file, err := cache.Get(ctx, 1, path)
			Expect(err).ToNot(HaveOccurred())
			Expect(file).ToNot(BeNil())
			Expect(file.ID).To(Equal(10))
			Expect(file.Storage).To(Equal(1))
			Expect(file.Path).To(Equal(path))
			Expect(file.Parent).To(Equal(9))
			Expect(file.MimeType).To(Equal(6))
			Expect(file.MimePart).To(Equal(5))
			Expect(file.MimeTypeString).To(Equal("image/jpeg"))
			Expect(file.Size).To(Equal(243733))
			Expect(file.MTime).To(Equal(1619007009))
			Expect(file.StorageMTime).To(Equal(1619007009))
			Expect(file.Encrypted).To(BeFalse())
			Expect(file.UnencryptedSize).To(Equal(0))
			Expect(file.Name).To(Equal("Portugal.jpg"))
			Expect(file.Etag).To(Equal("13cf411aefccd7183d3b117ccd0ac5f8"))
			Expect(file.Checksum).To(Equal("SHA1:872adcabcb4e06bea6265200c0d71b12defe2df1 MD5:01b38c622feac31652d738a94e15e86b ADLER32:6959358d"))
		})
	})

	Describe("List", func() {
		It("lists all entries", func() {
			list, err := cache.List(ctx, 1, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(list)).To(Equal(3))
		})

		It("filters", func() {
			list, err := cache.List(ctx, 1, "files_trashbin/")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(list)).To(Equal(3))
		})

		It("filters deep", func() {
			list, err := cache.List(ctx, 1, "files/Photos/")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(list)).To(Equal(3))
		})
	})

	Describe("Path", func() {
		It("returns the path", func() {
			path, err := cache.Path(ctx, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("files/Photos/Portugal.jpg"))
		})

		It("returns the path when given a string id", func() {
			path, err := cache.Path(ctx, "10")
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("files/Photos/Portugal.jpg"))
		})
	})

	Describe("InsertOrUpdate", func() {
		Context("when inserting a new recored", func() {
			It("checks for required fields", func() {
				data := map[string]interface{}{
					"mimetype": "httpd/unix-directory",
					"etag":     "abcdefg",
				}
				_, err := cache.InsertOrUpdate(ctx, 3, data, false)
				Expect(err).To(MatchError("missing required data"))

				data = map[string]interface{}{
					"path": "files/Photos/foo.jpg",
					"etag": "abcdefg",
				}
				_, err = cache.InsertOrUpdate(ctx, 3, data, false)
				Expect(err).To(MatchError("missing required data"))

				data = map[string]interface{}{
					"path":     "files/Photos/foo.jpg",
					"mimetype": "httpd/unix-directory",
				}
				_, err = cache.InsertOrUpdate(ctx, 3, data, false)
				Expect(err).To(MatchError("missing required data"))
			})

			It("inserts a new minimal entry", func() {
				data := map[string]interface{}{
					"path":     "files/Photos/foo.jpg",
					"mimetype": "httpd/unix-directory",
					"etag":     "abcdefg",
				}
				id, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(id).To(Equal(18))

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.jpg")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.Path).To(Equal("files/Photos/foo.jpg"))
				Expect(entry.Name).To(Equal("foo.jpg"))
				Expect(entry.MimeType).To(Equal(2))
				Expect(entry.MimePart).To(Equal(1))
				Expect(entry.Etag).To(Equal("abcdefg"))
			})

			It("inserts a complete entry", func() {
				data := map[string]interface{}{
					"path":             "files/Photos/foo.jpg",
					"checksum":         "SHA1: abcdefg",
					"etag":             "abcdefg",
					"size":             1234,
					"mimetype":         "image/jpeg",
					"mtime":            1617702482,
					"storage_mtime":    1617702483,
					"encrypted":        true,
					"unencrypted_size": 2000,
				}
				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.jpg")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.Path).To(Equal("files/Photos/foo.jpg"))
				Expect(entry.Name).To(Equal("foo.jpg"))
				Expect(entry.Checksum).To(Equal("SHA1: abcdefg"))
				Expect(entry.Etag).To(Equal("abcdefg"))
				Expect(entry.Size).To(Equal(1234))
				Expect(entry.MimeType).To(Equal(6))
				Expect(entry.MimePart).To(Equal(5))
				Expect(entry.MTime).To(Equal(1617702482))
				Expect(entry.StorageMTime).To(Equal(1617702483))
				Expect(entry.Encrypted).To(BeTrue())
				Expect(entry.UnencryptedSize).To(Equal(2000))
			})

			It("sets the parent", func() {
				data := map[string]interface{}{
					"path":     "files/Photos/foo.jpg",
					"mimetype": "httpd/unix-directory",
					"etag":     "abcdefg",
				}

				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.jpg")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.Parent).To(Equal(9))
			})

			It("sets the mtime storage_mtime if not set", func() {
				data := map[string]interface{}{
					"path":          "files/Photos/foo.jpg",
					"mimetype":      "httpd/unix-directory",
					"etag":          "abcdefg",
					"storage_mtime": 1617702483,
				}

				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.jpg")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.MTime).To(Equal(1617702483))
			})

			It("sets the mimetype and part ids from the mimetype string", func() {
				data := map[string]interface{}{
					"path":     "files/Photos/foo.jpg",
					"checksum": "SHA1: abcdefg",
					"etag":     "abcdefg",
					"mimetype": "image/jpeg",
				}

				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.jpg")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.MimeType).To(Equal(6))
				Expect(entry.MimePart).To(Equal(5))
			})

			It("adds unknown mimetypes to the database", func() {
				data := map[string]interface{}{
					"path":     "files/Photos/foo.tiff",
					"checksum": "SHA1: abcdefg",
					"etag":     "abcdefg",
					"mimetype": "image/tiff",
				}

				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				entry, err := cache.Get(ctx, 1, "files/Photos/foo.tiff")
				Expect(err).ToNot(HaveOccurred())
				Expect(entry.MimeType).To(Equal(9))
				Expect(entry.MimePart).To(Equal(5))
			})

			It("does not add a . as the name for root entries", func() {
				data := map[string]interface{}{
					"path":     "",
					"checksum": "SHA1: abcdefg",
					"etag":     "abcdefg",
					"mimetype": "image/tiff",
				}

				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())

				file, err := cache.Get(ctx, 1, "")
				Expect(err).ToNot(HaveOccurred())
				Expect(file).ToNot(BeNil())
				Expect(file.Name).To(Equal(""))
			})
		})

		Context("when updating an existing record", func() {
			var (
				data map[string]interface{}
			)

			BeforeEach(func() {
				data = map[string]interface{}{
					"path":     "files/Photos/foo.jpg",
					"mimetype": "httpd/unix-directory",
					"etag":     "abcdefg",
				}
				_, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("updates the record", func() {
				recordBefore, err := cache.Get(ctx, 1, data["path"].(string))
				Expect(err).ToNot(HaveOccurred())

				data["etag"] = "12345"
				id, err := cache.InsertOrUpdate(ctx, 1, data, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(id).To(Equal(recordBefore.ID))

				recordAfter, err := cache.Get(ctx, 1, data["path"].(string))
				Expect(err).ToNot(HaveOccurred())

				Expect(recordBefore.Etag).To(Equal("abcdefg"))
				Expect(recordAfter.Etag).To(Equal("12345"))
			})

		})
	})

	Describe("Move", func() {
		It("moves a file", func() {
			err := cache.Move(ctx, 1, "files/Photos/Portugal.jpg", "files/Documents/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())

			_, err = cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).To(HaveOccurred())

			newEntry, err := cache.Get(ctx, 1, "files/Documents/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(newEntry.Path).To(Equal("files/Documents/Portugal.jpg"))
		})

		It("moves a file while changing its name", func() {
			err := cache.Move(ctx, 1, "files/Photos/Portugal.jpg", "files/Documents/Spain.jpg")
			Expect(err).ToNot(HaveOccurred())

			_, err = cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).To(HaveOccurred())

			newEntry, err := cache.Get(ctx, 1, "files/Documents/Spain.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(newEntry.Path).To(Equal("files/Documents/Spain.jpg"))
			Expect(newEntry.Name).To(Equal("Spain.jpg"))
		})

		It("moves a directory", func() {
			err := cache.Move(ctx, 1, "files/Photos", "files/Foo")
			Expect(err).ToNot(HaveOccurred())

			_, err = cache.Get(ctx, 1, "files/Photos")
			Expect(err).To(HaveOccurred())

			_, err = cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).To(HaveOccurred())
			newEntry, err := cache.Get(ctx, 1, "files/Foo/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(newEntry.Path).To(Equal("files/Foo/Portugal.jpg"))
		})
	})

	Describe("SetEtag", func() {
		It("updates the etag", func() {
			entry, err := cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(entry.Etag).To(Equal("13cf411aefccd7183d3b117ccd0ac5f8"))

			err = cache.SetEtag(ctx, 1, "files/Photos/Portugal.jpg", "foo")
			Expect(err).ToNot(HaveOccurred())

			entry, err = cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(entry.Etag).To(Equal("foo"))
		})
	})

	Context("trash", func() {
		var (
			filePath = "files/Photos/Portugal.jpg"

			data = map[string]interface{}{
				"path":     "files_trashbin/files/Photos",
				"mimetype": "httpd/unix-directory",
				"etag":     "abcdefg",
			}
			trashPathBase      = "Portugal.jpg"
			trashPathTimestamp = 1619007109
			trashPath          = "files_trashbin/files/" + trashPathBase + ".d" + strconv.Itoa(trashPathTimestamp)
		)

		BeforeEach(func() {
			_, err := cache.InsertOrUpdate(ctx, 1, data, false)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("Delete", func() {
			It("deletes an item", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
				Expect(err).To(HaveOccurred())
				_, err = cache.Get(ctx, 1, "files_trashbin/files/Portugal.jpg.d1619007109")
				Expect(err).ToNot(HaveOccurred())
			})

			It("creates an entry in the trash table", func() {
				_, err := cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).To(HaveOccurred())

				err = cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				item, err := cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).ToNot(HaveOccurred())
				Expect(item.Path).To(Equal("Photos"))
			})

			It("rewrites the path of the children", func() {
				err := cache.Delete(ctx, 1, "admin", "files/Photos", "files_trashbin/files/Photos.d1619007109")
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("EmptyRecycle", func() {
			It("clears the recycle bin", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				err = cache.EmptyRecycle(ctx, "admin")
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("DeleteRecycleItem", func() {
			It("removes the item from the trash", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				err = cache.DeleteRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).To(HaveOccurred())
			})

			It("does not remove the item from the file cache", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				err = cache.DeleteRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.Get(ctx, 1, trashPath)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("PurgeRecycleItem", func() {
			It("removes the item from the database", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).ToNot(HaveOccurred())

				err = cache.PurgeRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp, false)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.GetRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp)
				Expect(err).To(HaveOccurred())
			})

			It("removes the item from the filecache table", func() {
				err := cache.Delete(ctx, 1, "admin", filePath, trashPath)
				Expect(err).ToNot(HaveOccurred())

				err = cache.PurgeRecycleItem(ctx, "admin", trashPathBase, trashPathTimestamp, false)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.Get(ctx, 1, trashPath)
				Expect(err).To(HaveOccurred())
			})

			It("removes children from the filecache table", func() {
				err := cache.Delete(ctx, 1, "admin", "files/Photos", "files_trashbin/files/Photos.d1619007109")
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.Get(ctx, 1, "files_trashbin/files/Photos.d1619007109/Portugal.jpg")
				Expect(err).ToNot(HaveOccurred())

				err = cache.PurgeRecycleItem(ctx, "admin", "Photos", 1619007109, false)
				Expect(err).ToNot(HaveOccurred())

				_, err = cache.Get(ctx, 1, "files_trashbin/files/Photos.d1619007109/Portugal.jpg")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Copy", func() {
		It("copies the entry", func() {
			for _, dir := range []string{"files_versions", "files_versions/Photos"} {
				parentData := map[string]interface{}{
					"path":     dir,
					"mimetype": "httpd/unix-directory",
					"etag":     "abcdefg",
				}
				_, err := cache.InsertOrUpdate(ctx, 1, parentData, false)
				Expect(err).ToNot(HaveOccurred())
			}

			existingEntry, err := cache.Get(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			_, err = cache.Copy(ctx, 1, "files/Photos/Portugal.jpg", "files_versions/Photos/Portugal.jpg.v1619528083")
			Expect(err).ToNot(HaveOccurred())

			newEntry, err := cache.Get(ctx, 1, "files_versions/Photos/Portugal.jpg.v1619528083")
			Expect(err).ToNot(HaveOccurred())
			Expect(newEntry.ID).ToNot(Equal(existingEntry.ID))
			Expect(newEntry.MimeType).To(Equal(existingEntry.MimeType))
		})
	})

	Describe("Permissions", func() {
		It("returns the permissions", func() {
			perms, err := cache.Permissions(ctx, 1, "files/Photos/Portugal.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).ToNot(BeNil())
			Expect(perms.InitiateFileUpload).To(BeTrue())

			perms, err = cache.Permissions(ctx, 1, "files/Photos/Teotihuacan.jpg")
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).ToNot(BeNil())
			Expect(perms.InitiateFileUpload).To(BeFalse())
		})
	})
})
