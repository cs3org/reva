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

package nextcloud_test

import (
	"context"
	// "fmt".
	"io"
	"net/url"
	"os"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/storage/fs/nextcloud"
	jwt "github.com/cs3org/reva/pkg/token/manager/jwt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/metadata"
)

func setUpNextcloudServer() (*nextcloud.StorageDriver, *[]string, func()) {
	var conf *nextcloud.StorageDriverConfig

	ncHost := os.Getenv("NEXTCLOUD")
	if len(ncHost) == 0 {
		conf = &nextcloud.StorageDriverConfig{
			EndPoint: "http://mock.com/apps/sciencemesh/",
			MockHTTP: true,
		}
		nc, _ := nextcloud.NewStorageDriver(conf)
		called := make([]string, 0)
		h := nextcloud.GetNextcloudServerMock(&called)
		mock, teardown := nextcloud.TestingHTTPClient(h)
		nc.SetHTTPClient(mock)
		return nc, &called, teardown
	}
	conf = &nextcloud.StorageDriverConfig{
		EndPoint: ncHost + "/apps/sciencemesh/",
		MockHTTP: false,
	}
	nc, _ := nextcloud.NewStorageDriver(conf)
	return nc, nil, func() {}
}

func checkCalled(called *[]string, expected string) {
	if called == nil {
		return
	}
	Expect(len(*called)).To(Equal(1))
	Expect((*called)[0]).To(Equal(expected))
}

var _ = Describe("Nextcloud", func() {
	var (
		ctx     context.Context
		options map[string]interface{}
		tmpRoot string
		user    = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "tester",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "tester",
		}
	)

	BeforeEach(func() {
		var err error

		options = map[string]interface{}{
			"endpoint":  "http://mock.com/apps/sciencemesh/",
			"mock_http": true,
		}

		ctx = context.Background()

		// Add auth token
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		scope, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user, scope)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, user)
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			_, err := nextcloud.New(options)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// 	GetHome(ctx context.Context) (string, error)
	Describe("GetHome", func() {
		It("calls the GetHome endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			home, err := nc.GetHome(ctx)
			Expect(home).To(Equal("yes we are"))
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/GetHome `)
		})
	})

	// CreateHome(ctx context.Context) error
	Describe("CreateHome", func() {
		It("calls the CreateHome endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			err := nc.CreateHome(ctx)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/CreateHome `)
		})
	})

	// CreateDir(ctx context.Context, ref *provider.Reference) error
	Describe("CreateDir", func() {
		It("calls the CreateDir endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some/path",
			}
			err := nc.CreateDir(ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/CreateDir {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`)
		})
	})

	// Delete(ctx context.Context, ref *provider.Reference) error
	Describe("Delete", func() {
		It("calls the Delete endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some/path",
			}
			err := nc.Delete(ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/Delete {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`)
		})
	})

	// Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	Describe("Move", func() {
		It("calls the Move endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref1 := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id-1",
					OpaqueId:  "opaque-id-1",
				},
				Path: "/some/old/path",
			}
			ref2 := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id-2",
					OpaqueId:  "opaque-id-2",
				},
				Path: "/some/new/path",
			}
			err := nc.Move(ctx, ref1, ref2)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/Move {"oldRef":{"resource_id":{"storage_id":"storage-id-1","opaque_id":"opaque-id-1"},"path":"/some/old/path"},"newRef":{"resource_id":{"storage_id":"storage-id-2","opaque_id":"opaque-id-2"},"path":"/some/new/path"}}`)
		})
	})

	// GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error)
	Describe("GetMD", func() {
		It("calls the GetMD endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some/path",
			}
			mdKeys := []string{"val1", "val2", "val3"}
			result, err := nc.GetMD(ctx, ref, mdKeys)
			Expect(err).ToNot(HaveOccurred())
			Expect(*result).To(Equal(provider.ResourceInfo{
				Opaque: &types.Opaque{
					Map:                  nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Type: provider.ResourceType_RESOURCE_TYPE_FILE,
				Id: &provider.ResourceId{
					StorageId:            "",
					OpaqueId:             "fileid-/some/path",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Checksum: &provider.ResourceChecksum{
					Type:                 0,
					Sum:                  "",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Etag:     "deadbeef",
				MimeType: "text/plain",
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Path: "/some/path",
				PermissionSet: &provider.ResourcePermissions{
					AddGrant:             false,
					CreateContainer:      false,
					Delete:               false,
					GetPath:              false,
					GetQuota:             false,
					InitiateFileDownload: false,
					InitiateFileUpload:   false,
					ListGrants:           false,
					ListContainer:        false,
					ListFileVersions:     false,
					ListRecycle:          false,
					Move:                 false,
					RemoveGrant:          false,
					PurgeRecycle:         false,
					RestoreFileVersion:   false,
					RestoreRecycleItem:   false,
					Stat:                 false,
					UpdateGrant:          false,
					DenyGrant:            false,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Size:   12345,
				Owner:  nil,
				Target: "",
				CanonicalMetadata: &provider.CanonicalMetadata{
					Target:               nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				ArbitraryMetadata: &provider.ArbitraryMetadata{
					Metadata:             map[string]string{"some": "arbi", "trary": "meta", "da": "ta"},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/GetMD {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"},"mdKeys":["val1","val2","val3"]}`)
		})
	})

	// ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error)
	Describe("ListFolder", func() {
		It("calls the ListFolder endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some",
			}
			mdKeys := []string{"val1", "val2", "val3"}
			results, err := nc.ListFolder(ctx, ref, mdKeys)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(Equal(1))
			Expect(*results[0]).To(Equal(provider.ResourceInfo{
				Opaque: &types.Opaque{
					Map:                  nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Type: provider.ResourceType_RESOURCE_TYPE_FILE,
				Id: &provider.ResourceId{
					StorageId:            "",
					OpaqueId:             "fileid-/some/path",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Checksum: &provider.ResourceChecksum{
					Type:                 0,
					Sum:                  "",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Etag:     "deadbeef",
				MimeType: "text/plain",
				Mtime: &types.Timestamp{
					Seconds:              1234567890,
					Nanos:                0,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Path: "/some/path",
				PermissionSet: &provider.ResourcePermissions{
					AddGrant:             false,
					CreateContainer:      false,
					Delete:               false,
					GetPath:              false,
					GetQuota:             false,
					InitiateFileDownload: false,
					InitiateFileUpload:   false,
					ListGrants:           false,
					ListContainer:        false,
					ListFileVersions:     false,
					ListRecycle:          false,
					Move:                 false,
					RemoveGrant:          false,
					PurgeRecycle:         false,
					RestoreFileVersion:   false,
					RestoreRecycleItem:   false,
					Stat:                 false,
					UpdateGrant:          false,
					DenyGrant:            false,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Size:   12345,
				Owner:  nil,
				Target: "",
				CanonicalMetadata: &provider.CanonicalMetadata{
					Target:               nil,
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				ArbitraryMetadata: &provider.ArbitraryMetadata{
					Metadata:             map[string]string{"some": "arbi", "trary": "meta", "da": "ta"},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}))
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/ListFolder {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some"},"mdKeys":["val1","val2","val3"]}`)
		})
	})

	// InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error)
	Describe("InitiateUpload", func() {
		It("calls the InitiateUpload endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some/path",
			}
			uploadLength := int64(12345)
			metadata := map[string]string{
				"key1": "val1",
				"key2": "val2",
				"key3": "val3",
			}
			results, err := nc.InitiateUpload(ctx, ref, uploadLength, metadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(Equal(map[string]string{
				"not":      "sure",
				"what":     "should be",
				"returned": "here",
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/InitiateUpload {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"},"uploadLength":12345,"metadata":{"key1":"val1","key2":"val2","key3":"val3"}}`)
		})
	})

	// Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error
	Describe("Upload", func() {
		It("calls the Upload endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			stringReader := strings.NewReader("shiny!")
			stringReadCloser := io.NopCloser(stringReader)
			err := nc.Upload(ctx, ref, stringReadCloser)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `PUT /apps/sciencemesh/~tester/api/storage/Upload/some/file/path.txt shiny!`)
		})
	})
	// Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error)
	Describe("Download", func() {
		It("calls the Download endpoint with GET", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			reader, err := nc.Download(ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `GET /apps/sciencemesh/~tester/api/storage/Download/some/file/path.txt `)
			defer reader.Close()
			body, err := io.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(Equal("the contents of the file"))
		})
	})

	// ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	Describe("ListRevisions", func() {
		It("calls the ListRevisions endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "/some/path",
			}
			results, err := nc.ListRevisions(ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L1003-L1023
			Expect(len(results)).To(Equal(2))
			Expect(*results[0]).To(Equal(provider.FileVersion{
				Opaque: &types.Opaque{
					Map: map[string]*types.OpaqueEntry{
						"some": {
							Value: []byte("data"),
						},
					},
				},
				Key:                  "version-12",
				Size:                 uint64(12345),
				Mtime:                uint64(1234567890),
				Etag:                 "deadb00f",
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}))
			Expect(*results[1]).To(Equal(provider.FileVersion{
				Opaque: &types.Opaque{
					Map: map[string]*types.OpaqueEntry{
						"different": {
							Value: []byte("stuff"),
						},
					},
				},
				Key:                  "asdf",
				Size:                 uint64(12345),
				Mtime:                uint64(1234567890),
				Etag:                 "deadbeef",
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/ListRevisions {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"/some/path"}`)
		})
	})

	// DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	Describe("DownloadRevision", func() {
		It("calls the DownloadRevision endpoint with GET", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			key := "some/revision"
			reader, err := nc.DownloadRevision(ctx, ref, key)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `GET /apps/sciencemesh/~tester/api/storage/DownloadRevision/some%2Frevision/some/file/path.txt `)
			defer reader.Close()
			body, err := io.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(Equal("the contents of that revision"))
		})
	})

	// RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	Describe("RestoreRevision", func() {
		It("calls the RestoreRevision endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			key := "asdf"
			err := nc.RestoreRevision(ctx, ref, key)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/RestoreRevision {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"key":"asdf"}`)
		})
	})

	// ListRecycle(ctx context.Context, key, path string) ([]*provider.RecycleItem, error)
	Describe("ListRecycle", func() {
		It("calls the ListRecycle endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()

			results, err := nc.ListRecycle(ctx, "/", "asdf", "/some/file.txt")
			Expect(err).ToNot(HaveOccurred())
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L1085-L1110
			Expect(len(results)).To(Equal(1))
			Expect(*results[0]).To(Equal(provider.RecycleItem{
				Opaque: &types.Opaque{},
				Key:    "some-deleted-version",
				Ref: &provider.Reference{
					ResourceId:           &provider.ResourceId{},
					Path:                 "/some/file.txt",
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     nil,
					XXX_sizecache:        0,
				},
				Size:                 uint64(12345),
				DeletionTime:         &types.Timestamp{Seconds: uint64(1234567890)},
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/ListRecycle {"key":"asdf","path":"/some/file.txt"}`)
		})
	})

	// RestoreRecycleItem(ctx context.Context, key, path string, restoreRef *provider.Reference) error
	Describe("RestoreRecycleItem", func() {
		It("calls the RestoreRecycleItem endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L550-L561
			restoreRef := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			path := "original/location/when/deleted.txt"
			key := "asdf"
			err := nc.RestoreRecycleItem(ctx, "/", key, path, restoreRef)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/RestoreRecycleItem {"key":"asdf","path":"original/location/when/deleted.txt","restoreRef":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"}}`)
		})
	})
	// PurgeRecycleItem(ctx context.Context, key, path string) error
	Describe("PurgeRecycleItem", func() {
		It("calls the PurgeRecycleItem endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			path := "original/location/when/deleted.txt"
			key := "asdf"
			err := nc.PurgeRecycleItem(ctx, "/", key, path)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/PurgeRecycleItem {"key":"asdf","path":"original/location/when/deleted.txt"}`)
		})
	})

	// EmptyRecycle(ctx context.Context) error
	Describe("EmpytRecycle", func() {
		It("calls the EmpytRecycle endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			err := nc.EmptyRecycle(ctx)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/EmptyRecycle `)
		})
	})

	// GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	Describe("GetPathByID", func() {
		It("calls the GetPathByID endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L602-L618
			id := &provider.ResourceId{
				StorageId: "storage-id",
				OpaqueId:  "opaque-id",
			}
			path, err := nc.GetPathByID(ctx, id)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/GetPathByID {"storage_id":"storage-id","opaque_id":"opaque-id"}`)
			Expect(path).To(Equal("the/path/for/that/id.txt"))
		})
	})

	// AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	Describe("AddGrant", func() {
		It("calls the AddGrant endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L843-L855
			grant := &provider.Grant{
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L896-L915
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L659-L683
				Permissions: &provider.ResourcePermissions{
					AddGrant:             true,
					CreateContainer:      true,
					Delete:               true,
					GetPath:              true,
					GetQuota:             true,
					InitiateFileDownload: true,
					InitiateFileUpload:   true,
					ListGrants:           true,
					ListContainer:        true,
					ListFileVersions:     true,
					ListRecycle:          true,
					Move:                 true,
					RemoveGrant:          true,
					PurgeRecycle:         true,
					RestoreFileVersion:   true,
					RestoreRecycleItem:   true,
					Stat:                 true,
					UpdateGrant:          true,
					DenyGrant:            true,
				},
			}
			err := nc.AddGrant(ctx, ref, grant)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/AddGrant {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"g":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`)
		})
	})

	// DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error
	Describe("DenyGrant", func() {
		It("calls the DenyGrant endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L896-L915
			grantee := &provider.Grantee{
				Id: &provider.Grantee_UserId{
					UserId: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
				},
			}
			err := nc.DenyGrant(ctx, ref, grantee)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/DenyGrant {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"g":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}}}`)
		})
	})

	// RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	Describe("RemoveGrant", func() {
		It("calls the RemoveGrant endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L843-L855
			grant := &provider.Grant{
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L896-L915
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L659-L683
				Permissions: &provider.ResourcePermissions{
					AddGrant:             true,
					CreateContainer:      true,
					Delete:               true,
					GetPath:              true,
					GetQuota:             true,
					InitiateFileDownload: true,
					InitiateFileUpload:   true,
					ListGrants:           true,
					ListContainer:        true,
					ListFileVersions:     true,
					ListRecycle:          true,
					Move:                 true,
					RemoveGrant:          true,
					PurgeRecycle:         true,
					RestoreFileVersion:   true,
					RestoreRecycleItem:   true,
					Stat:                 true,
					UpdateGrant:          true,
					DenyGrant:            true,
				},
			}
			err := nc.RemoveGrant(ctx, ref, grant)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/RemoveGrant {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"g":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`)
		})
	})

	// UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	Describe("UpdateGrant", func() {
		It("calls the UpdateGrant endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L843-L855
			grant := &provider.Grant{
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L896-L915
				Grantee: &provider.Grantee{
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{
							Idp:      "0.0.0.0:19000",
							OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
				},
				// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L659-L683
				Permissions: &provider.ResourcePermissions{
					AddGrant:             true,
					CreateContainer:      true,
					Delete:               true,
					GetPath:              true,
					GetQuota:             true,
					InitiateFileDownload: true,
					InitiateFileUpload:   true,
					ListGrants:           true,
					ListContainer:        true,
					ListFileVersions:     true,
					ListRecycle:          true,
					Move:                 true,
					RemoveGrant:          true,
					PurgeRecycle:         true,
					RestoreFileVersion:   true,
					RestoreRecycleItem:   true,
					Stat:                 true,
					UpdateGrant:          true,
					DenyGrant:            true,
				},
			}
			err := nc.UpdateGrant(ctx, ref, grant)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/UpdateGrant {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"g":{"grantee":{"Id":{"UserId":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true,"list_container":true,"list_file_versions":true,"list_recycle":true,"move":true,"remove_grant":true,"purge_recycle":true,"restore_file_version":true,"restore_recycle_item":true,"stat":true,"update_grant":true,"deny_grant":true}}}`)
		})
	})

	// ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	Describe("ListGrants", func() {
		It("calls the ListGrants endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			grants, err := nc.ListGrants(ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(grants)).To(Equal(1))

			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/ListGrants {"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"}`)
		})
	})

	// GetQuota(ctx context.Context) (uint64, uint64, error)
	Describe("GetQuota", func() {
		It("calls the GetQuota endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			maxBytes, maxFiles, err := nc.GetQuota(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(maxBytes).To(Equal(uint64(456)))
			Expect(maxFiles).To(Equal(uint64(123)))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/GetQuota `)
		})
	})

	// CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Describe("CreateReference", func() {
		It("calls the CreateReference endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			path := "some/file/path.txt"
			targetURI, err := url.Parse("http://bing.com/search?q=dotnet")
			Expect(err).ToNot(HaveOccurred())
			err = nc.CreateReference(ctx, path, targetURI)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/CreateReference {"path":"some/file/path.txt","url":"http://bing.com/search?q=dotnet"}`)
		})
	})

	// Shutdown(ctx context.Context) error
	Describe("Shutdown", func() {
		It("calls the Shutdown endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			err := nc.Shutdown(ctx)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/Shutdown `)
		})
	})

	// SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	Describe("SetArbitraryMetadata", func() {
		It("calls the SetArbitraryMetadata endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			md := &provider.ArbitraryMetadata{
				Metadata: map[string]string{
					"arbi": "trary",
					"meta": "data",
				},
			}
			err := nc.SetArbitraryMetadata(ctx, ref, md)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/SetArbitraryMetadata {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"md":{"metadata":{"arbi":"trary","meta":"data"}}}`)
		})
	})

	// UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
	Describe("UnsetArbitraryMetadata", func() {
		It("calls the UnsetArbitraryMetadata endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storage-id",
					OpaqueId:  "opaque-id",
				},
				Path: "some/file/path.txt",
			}
			keys := []string{"arbi"}
			err := nc.UnsetArbitraryMetadata(ctx, ref, keys)
			Expect(err).ToNot(HaveOccurred())
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/UnsetArbitraryMetadata {"ref":{"resource_id":{"storage_id":"storage-id","opaque_id":"opaque-id"},"path":"some/file/path.txt"},"keys":["arbi"]}`)
		})
	})

	// ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error)
	Describe("ListStorageSpaces", func() {
		It("calls the ListStorageSpaces endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			filter1 := &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_OWNER,
				Term: &provider.ListStorageSpacesRequest_Filter_Owner{
					Owner: &userpb.UserId{
						Idp:      "0.0.0.0:19000",
						OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
				},
			}
			filter2 := &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &provider.ListStorageSpacesRequest_Filter_Id{
					Id: &provider.StorageSpaceId{
						OpaqueId: "opaque-id",
					},
				},
			}
			filter3 := &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &provider.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: string("home"),
				},
			}
			filters := []*provider.ListStorageSpacesRequest_Filter{filter1, filter2, filter3}
			spaces, err := nc.ListStorageSpaces(ctx, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(spaces)).To(Equal(1))
			// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L1341-L1366
			Expect(*spaces[0]).To(Equal(provider.StorageSpace{
				Opaque: &types.Opaque{
					Map: map[string](*types.OpaqueEntry){
						"foo": &types.OpaqueEntry{Value: []byte("sama")},
						"bar": &types.OpaqueEntry{Value: []byte("sama")},
					},
				},
				Id: &provider.StorageSpaceId{OpaqueId: "some-opaque-storage-space-id"},
				Owner: &userpb.User{
					Id: &userpb.UserId{
						Idp:      "some-idp",
						OpaqueId: "some-opaque-user-id",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
				},
				Root: &provider.ResourceId{
					StorageId: "some-storage-ud",
					OpaqueId:  "some-opaque-root-id",
				},
				Name: "My Storage Space",
				Quota: &provider.Quota{
					QuotaMaxBytes: uint64(456),
					QuotaMaxFiles: uint64(123),
				},
				SpaceType: "home",
				Mtime: &types.Timestamp{
					Seconds: uint64(1234567890),
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/ListStorageSpaces [{"type":3,"Term":{"Owner":{"idp":"0.0.0.0:19000","opaque_id":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c","type":1}}},{"type":2,"Term":{"Id":{"opaque_id":"opaque-id"}}},{"type":4,"Term":{"SpaceType":"home"}}]`)
		})
	})

	// CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error)
	Describe("CreateStorageSpace", func() {
		It("calls the CreateStorageSpace endpoint", func() {
			nc, called, teardown := setUpNextcloudServer()
			defer teardown()
			// https://github.com/cs3org/go-cs3apis/blob/03e4a408c1f3b2882916cf3fad4c71081a20711d/cs3/storage/provider/v1beta1/provider_api.pb.go#L3176-L3192
			result, err := nc.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
				Opaque: &types.Opaque{
					Map: map[string](*types.OpaqueEntry){
						"foo": &types.OpaqueEntry{Value: []byte("sama")},
						"bar": &types.OpaqueEntry{Value: []byte("sama")},
					},
				},
				Owner: &userpb.User{
					Id: &userpb.UserId{
						Idp:      "some-idp",
						OpaqueId: "some-opaque-user-id",
						Type:     userpb.UserType_USER_TYPE_PRIMARY,
					},
				},
				Name: "My Storage Space",
				Quota: &provider.Quota{
					QuotaMaxBytes: uint64(456),
					QuotaMaxFiles: uint64(123),
				},
				Type: "home",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(*result).To(Equal(provider.CreateStorageSpaceResponse{
				Opaque: nil,
				Status: nil,
				StorageSpace: &provider.StorageSpace{
					Opaque: &types.Opaque{
						Map: map[string](*types.OpaqueEntry){
							"bar": &types.OpaqueEntry{Value: []byte("sama")},
							"foo": &types.OpaqueEntry{Value: []byte("sama")},
						},
					},
					Id: &provider.StorageSpaceId{OpaqueId: "some-opaque-storage-space-id"},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							Idp:      "some-idp",
							OpaqueId: "some-opaque-user-id",
							Type:     userpb.UserType_USER_TYPE_PRIMARY,
						},
					},
					Root: &provider.ResourceId{
						StorageId: "some-storage-ud",
						OpaqueId:  "some-opaque-root-id",
					},
					Name: "My Storage Space",
					Quota: &provider.Quota{
						QuotaMaxBytes: uint64(456),
						QuotaMaxFiles: uint64(123),
					},
					SpaceType: "home",
					Mtime: &types.Timestamp{
						Seconds: uint64(1234567890),
					},
				},
			}))
			checkCalled(called, `POST /apps/sciencemesh/~tester/api/storage/CreateStorageSpace {"opaque":{"map":{"bar":{"value":"c2FtYQ=="},"foo":{"value":"c2FtYQ=="}}},"owner":{"id":{"idp":"some-idp","opaque_id":"some-opaque-user-id","type":1}},"type":"home","name":"My Storage Space","quota":{"quota_max_bytes":456,"quota_max_files":123}}`)
		})
	})

})
