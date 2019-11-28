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
	"net/url"

	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
)

// FS is the interface to implement access to the storage.
type FS interface {
	CreateDir(ctx context.Context, fn string) error
	Delete(ctx context.Context, ref *storageproviderv1beta1pb.Reference) error
	Move(ctx context.Context, oldRef, newRef *storageproviderv1beta1pb.Reference) error
	GetMD(ctx context.Context, ref *storageproviderv1beta1pb.Reference) (*storageproviderv1beta1pb.ResourceInfo, error)
	ListFolder(ctx context.Context, ref *storageproviderv1beta1pb.Reference) ([]*storageproviderv1beta1pb.ResourceInfo, error)
	Upload(ctx context.Context, ref *storageproviderv1beta1pb.Reference, r io.ReadCloser) error
	Download(ctx context.Context, ref *storageproviderv1beta1pb.Reference) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, ref *storageproviderv1beta1pb.Reference) ([]*storageproviderv1beta1pb.FileVersion, error)
	DownloadRevision(ctx context.Context, ref *storageproviderv1beta1pb.Reference, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, ref *storageproviderv1beta1pb.Reference, key string) error
	ListRecycle(ctx context.Context) ([]*storageproviderv1beta1pb.RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, key string) error
	PurgeRecycleItem(ctx context.Context, key string) error
	EmptyRecycle(ctx context.Context) error
	GetPathByID(ctx context.Context, id *storageproviderv1beta1pb.ResourceId) (string, error)
	AddGrant(ctx context.Context, ref *storageproviderv1beta1pb.Reference, g *storageproviderv1beta1pb.Grant) error
	RemoveGrant(ctx context.Context, ref *storageproviderv1beta1pb.Reference, g *storageproviderv1beta1pb.Grant) error
	UpdateGrant(ctx context.Context, ref *storageproviderv1beta1pb.Reference, g *storageproviderv1beta1pb.Grant) error
	ListGrants(ctx context.Context, ref *storageproviderv1beta1pb.Reference) ([]*storageproviderv1beta1pb.Grant, error)
	GetQuota(ctx context.Context) (int, int, error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Shutdown(ctx context.Context) error
	SetArbitraryMetadata(ctx context.Context, ref *storageproviderv1beta1pb.Reference, md *storageproviderv1beta1pb.ArbitraryMetadata) error
	UnsetArbitraryMetadata(ctx context.Context, ref *storageproviderv1beta1pb.Reference, keys []string) error
}

// Registry is the interface that storage registries implement
// for discovering storage providers
type Registry interface {
	FindProvider(ctx context.Context, ref *storageproviderv1beta1pb.Reference) (*storagetypespb.ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*storagetypespb.ProviderInfo, error)
	GetHome(ctx context.Context) (string, error)
}

// PathWrapper is the interface to implement for path transformations
type PathWrapper interface {
	Unwrap(ctx context.Context, rp string) (string, error)
	Wrap(ctx context.Context, rp string) (string, error)
}
