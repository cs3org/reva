// Copyright 2018-2024 CERN
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

package storage

import (
	"context"
	"fmt"
	"io"
	"net/textproto"
	"net/url"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// FS is the interface to implement access to the storage.
type FS interface {
	GetHome(ctx context.Context) (string, error)
	CreateHome(ctx context.Context) error
	CreateDir(ctx context.Context, ref *provider.Reference) error
	TouchFile(ctx context.Context, ref *provider.Reference) error
	Delete(ctx context.Context, ref *provider.Reference) error
	Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error)
	ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error)
	InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error)
	Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error
	Download(ctx context.Context, ref *provider.Reference, ranges []Range) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error
	PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error
	EmptyRecycle(ctx context.Context) error
	GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error
	RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	GetQuota(ctx context.Context, ref *provider.Reference) ( /*TotalBytes*/ uint64 /*UsedBytes*/, uint64, error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Shutdown(ctx context.Context) error
	SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
	SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error
	GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error)
	RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error
	Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error
	ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error)
	CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error)
	UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error)
}

// Should probably find a better place to put these, though not sure where
type Range struct {
	Start, Length int64
}

// ContentRange formats a Range header string as per RFC 7233.
func (r Range) ContentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.Start, r.Start+r.Length-1, size)
}

// MimeHeader creates range relevant MimeHeaders.
func (r Range) MimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.ContentRange(size)},
		"Content-Type":  {contentType},
	}
}

// Registry is the interface that storage registries implement
// for discovering storage providers.
type Registry interface {
	FindProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*registry.ProviderInfo, error)
	GetHome(ctx context.Context) (*registry.ProviderInfo, error)
}

// Route maps a path to the mount that serves it.
type Route struct {
	Path    string `json:"path"`
	MountID string `json:"mount_id"`
}

// Mount is a mount with the address of its provider.
type Mount struct {
	MountID   string `json:"mount_id"`
	MountPath string `json:"mount_path"`
	Address   string `json:"address"`
}

// RouteAdmin is implemented by a Registry with a routing table that an operator
// can change. AddRoute and RemoveRoute change only the table. Reload applies it,
// and each instance must reload.
type RouteAdmin interface {
	ListRoutes(ctx context.Context) ([]Route, error)
	ResolvePath(ctx context.Context, path string) ([]Mount, error)
	AddRoute(ctx context.Context, path, mountID string) error
	RemoveRoute(ctx context.Context, path string) error
	Reload(ctx context.Context) error
}

// PathWrapper is the interface to implement for path transformations.
type PathWrapper interface {
	Unwrap(ctx context.Context, rp string) (string, error)
	Wrap(ctx context.Context, rp string) (string, error)
}

// SpaceQuotaRef identifies the space of a quota.
type SpaceQuotaRef struct {
	// Root is the root of the space, relative to the storage.
	Root  string
	Type  spaces.SpaceType
	Owner *userpb.UserId
}

// SpaceQuota is the quota of a space. The bytes are logical, as users see them.
type SpaceQuota struct {
	// Root is relative to the storage.
	Root                string
	MaxBytes, UsedBytes uint64
	MaxFiles, UsedFiles uint64
	// PhysicalMaxBytes is MaxBytes with the copies that the storage keeps.
	PhysicalMaxBytes uint64
	// Form tells how the storage keeps this quota.
	Form string
}

// QuotaAdmin is implemented by a storage that lets administrators read and set
// the quota of any space it serves, whoever the user of the context is.
type QuotaAdmin interface {
	GetSpaceQuota(ctx context.Context, ref SpaceQuotaRef) (*SpaceQuota, error)
	// SetSpaceQuota returns the quota as the storage reports it after the
	// change.
	SetSpaceQuota(ctx context.Context, ref SpaceQuotaRef, maxBytes, maxFiles uint64) (*SpaceQuota, error)
	ListSpaceQuotas(ctx context.Context) ([]*SpaceQuota, error)
}
