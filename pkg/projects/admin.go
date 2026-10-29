// Copyright 2018-2026 CERN
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

package projects

import (
	"context"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Record is a project of the catalogue as an operator sees it.
type Record struct {
	Space  *provider.StorageSpace
	Status ProjectStatus
	// Extra holds what a driver stores beyond the generic project. The keys
	// belong to the driver.
	Extra map[string]string
}

// Ref selects one project by space id or by name. A name selects the
// project that is not archived, else the project archived last, because
// an archived project leaves its name free.
type Ref struct {
	SpaceID string
	Name    string
}

// Filter selects projects. A zero field does not filter.
type Filter struct {
	Status ProjectStatus
	Owner  string
	// Member keeps the projects that this user owns or can access through
	// a role group. Its Groups must be set.
	Member          *userpb.User
	CreatedAfter    time.Time
	CreatedBefore   time.Time
	IncludeArchived bool
}

// Registration is a project to add to the catalogue. The driver can fill
// an empty field from its configuration, or refuse it.
type Registration struct {
	Name        string
	Owner       string
	Path        string
	StorageID   string
	SpaceID     string
	Readers     string
	Writers     string
	Admins      string
	Description string
	Status      ProjectStatus
}

// Admin is the operator surface of a Catalogue. Unlike the Catalogue, it
// shows every project, whoever the caller is, so only the Admin API may
// use it. Every mutation invalidates the cache of this process only.
type Admin interface {
	ListProjectRecords(ctx context.Context, f Filter) ([]*Record, error)
	ProjectRecord(ctx context.Context, ref Ref) (*Record, error)
	SetProjectStatus(ctx context.Context, ref Ref, status ProjectStatus) (*Record, error)
	SetProjectOwner(ctx context.Context, ref Ref, owner string) (*Record, error)
	RegisterProject(ctx context.Context, r Registration) (*Record, error)
	// UnregisterProject soft-deletes the record, which keeps its name and
	// space id taken, or removes the row if hard is set.
	UnregisterProject(ctx context.Context, ref Ref, hard bool) (*Record, error)
	InvalidateCache()
}
