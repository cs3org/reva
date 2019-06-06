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
	"fmt"
	"io"

	"github.com/cs3org/reva/pkg/user"
)

// GranteeType specifies the type of grantee.
type GranteeType uint32

func (g GranteeType) String() string {
	switch g {
	case GranteeTypeUser:
		return "user"
	case GranteeTypeGroup:
		return "group"
	default:
		return fmt.Sprintf("invalid: %d", g)
	}
}

const (
	// GranteeTypeInvalid specifies an invalid permission.
	GranteeTypeInvalid = GranteeType(0)
	// GranteeTypeUser specifies the grantee is an individual user.
	GranteeTypeUser = GranteeType(1)
	// GranteeTypeGroup specifies the grantee is a group.
	GranteeTypeGroup = GranteeType(2)
)

// FS is the interface to implement access to the storage.
type FS interface {
	CreateDir(ctx context.Context, fn string) error
	Delete(ctx context.Context, fn string) error
	Move(ctx context.Context, old, new string) error
	GetMD(ctx context.Context, fn string) (*MD, error)
	ListFolder(ctx context.Context, fn string) ([]*MD, error)
	Upload(ctx context.Context, fn string, r io.ReadCloser) error
	Download(ctx context.Context, fn string) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, fn string) ([]*Revision, error)
	DownloadRevision(ctx context.Context, fn, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, fn, key string) error
	ListRecycle(ctx context.Context, fn string) ([]*RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, fn, key string) error
	EmptyRecycle(ctx context.Context, fn string) error
	GetPathByID(ctx context.Context, id string) (string, error)
	AddGrant(ctx context.Context, fn string, g *Grant) error
	RemoveGrant(ctx context.Context, fn string, g *Grant) error
	UpdateGrant(ctx context.Context, fn string, g *Grant) error
	ListGrants(ctx context.Context, fn string) ([]*Grant, error)
	GetQuota(ctx context.Context) (int, int, error)

	// Shutdown will be called when the service is being stopped.
	// Use it to properly
	// - shutdown embedded databases
	// - remove file listeners
	// TODO pass in context or log
	Shutdown() error
}

// MD represents the metadata about a file/directory.
type MD struct {
	ID          string // TODO use resourceID?
	Path        string
	Size        uint64
	Mtime       *Timestamp
	IsDir       bool
	Etag        string
	Checksum    string
	Mime        string
	Permissions *PermissionSet
	Opaque      map[string]interface{}
}

// Timestamp allows passing around a timestamp with sub second precision
type Timestamp struct {
	Seconds uint64
	Nanos   uint32
}

// PermissionSet is the set of permissions for a resource.
type PermissionSet struct {
	ListContainer   bool
	CreateContainer bool
	Move            bool
	Delete          bool
}

// Grant represents a grant for the storage.
type Grant struct {
	Grantee       *Grantee
	PermissionSet *PermissionSet
}

// Grantee is the receiver of the grant.
type Grantee struct {
	UserID *user.ID
	Type   GranteeType
}

// RecycleItem represents an entry in the recycle bin of the user.
type RecycleItem struct {
	RestorePath string
	RestoreKey  string
	Size        uint64
	DelMtime    uint64
	IsDir       bool
}

// Revision represents a version of the file in the past.
type Revision struct {
	RevKey string
	Size   uint64
	Mtime  uint64
	IsDir  bool
}

// Broker is the interface that storage brokers implement
// for discovering storage providers
type Broker interface {
	FindProvider(ctx context.Context, fn string) (*ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*ProviderInfo, error)
}

// ProviderInfo contains the information
// about a StorageProvider
type ProviderInfo struct {
	MountPath string
	Endpoint  string
}

// ResourceID identifies uniquely a resource
// across the distributed storage namespace.
type ResourceID struct {
	StorageID string
	OpaqueID  string
}
