// Copyright 2018-2019 CERN
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

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
)

// FS is the interface to implement access to the storage.
type FS interface {
	CreateDir(ctx context.Context, fn string) error
	Delete(ctx context.Context, ref *storageproviderv0alphapb.Reference) error
	Move(ctx context.Context, oldRef, newRef *storageproviderv0alphapb.Reference) error
	GetMD(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storageproviderv0alphapb.ResourceInfo, error)
	ListFolder(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.ResourceInfo, error)
	Upload(ctx context.Context, ref *storageproviderv0alphapb.Reference, r io.ReadCloser) error
	Download(ctx context.Context, ref *storageproviderv0alphapb.Reference) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.FileVersion, error)
	DownloadRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, key string) error
	ListRecycle(ctx context.Context) ([]*storageproviderv0alphapb.RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, key string) error
	EmptyRecycle(ctx context.Context) error
	GetPathByID(ctx context.Context, id *storageproviderv0alphapb.ResourceId) (string, error)
	AddGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error
	RemoveGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error
	UpdateGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error
	ListGrants(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.Grant, error)
	GetQuota(ctx context.Context) (int, int, error)
	Shutdown(ctx context.Context) error
}

// Registry is the interface that storage registries implement
// for discovering storage providers
type Registry interface {
	FindProvider(ctx context.Context, fn string) (*storagetypespb.ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*storagetypespb.ProviderInfo, error)
}
