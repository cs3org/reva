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

package storage

import (
	"context"
	"io"
	"net/url"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
)

// FS is the interface to implement access to the storage.
type FS interface {
	GetHome(ctx context.Context) (string, error)
	CreateHome(ctx context.Context) error
	CreateDir(ctx context.Context, fn string) error
	Delete(ctx context.Context, ref *provider.Reference) error
	Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error)
	ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error)
	InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error)
	Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error
	Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, key, restorePath string) error
	PurgeRecycleItem(ctx context.Context, key string) error
	EmptyRecycle(ctx context.Context) error
	GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	GetQuota(ctx context.Context) (uint64, uint64, error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Shutdown(ctx context.Context) error
	SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
}

// Registry is the interface that storage registries implement
// for discovering storage providers
type Registry interface {
	FindProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*registry.ProviderInfo, error)
	GetHome(ctx context.Context) (*registry.ProviderInfo, error)
}

// PathWrapper is the interface to implement for path transformations
type PathWrapper interface {
	Unwrap(ctx context.Context, rp string) (string, error)
	Wrap(ctx context.Context, rp string) (string, error)
}
