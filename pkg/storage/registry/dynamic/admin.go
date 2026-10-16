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

package dynamic

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// Route is one row of the routing table: a path served by a mount.
type Route struct {
	Path    string
	MountID string
}

// RouteAdmin is the operator surface of the dynamic registry, used by the
// storageregistry's admin invocations: the routing table is read once at
// start-up, so a change has to go through here (or be followed by Reload) to
// take effect without a restart.
type RouteAdmin interface {
	ListRoutes(ctx context.Context) ([]Route, error)
	AddRoute(ctx context.Context, path, mountID string) error
	RemoveRoute(ctx context.Context, path string) error
	Reload(ctx context.Context) error
}

// ListRoutes reads the routing table, by path.
func (d *dynamic) ListRoutes(ctx context.Context) ([]Route, error) {
	return readRoutes(ctx, d.db)
}

// AddRoute adds or updates the route of a path and reloads the tree. The
// mount must be one of the configured rules, as a route to an unknown mount
// would be skipped at load time.
func (d *dynamic) AddRoute(ctx context.Context, path, mountID string) error {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("dynamic: a route path must be absolute, not %q", path)
	}
	if _, ok := d.r[mountID]; !ok {
		return fmt.Errorf("dynamic: mount %q is not configured in the rules of this registry", mountID)
	}
	res, err := d.db.ExecContext(ctx, "UPDATE routing SET mount_id = ? WHERE path = ?", mountID, path)
	if err != nil {
		return errors.Wrap(err, "dynamic: updating the route")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := d.db.ExecContext(ctx, "INSERT INTO routing (path, mount_id) VALUES (?, ?)", path, mountID); err != nil {
			return errors.Wrap(err, "dynamic: inserting the route")
		}
	}
	return d.Reload(ctx)
}

// RemoveRoute deletes the route of a path and reloads the tree.
func (d *dynamic) RemoveRoute(ctx context.Context, path string) error {
	res, err := d.db.ExecContext(ctx, "DELETE FROM routing WHERE path = ?", strings.TrimSpace(path))
	if err != nil {
		return errors.Wrap(err, "dynamic: deleting the route")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("dynamic: no route for %q", path)
	}
	return d.Reload(ctx)
}

// Reload rebuilds the routing tree from the table.
func (d *dynamic) Reload(ctx context.Context) error {
	rt, err := loadRoutingTree(ctx, d.db, d.r)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.rt = rt
	d.mu.Unlock()
	return nil
}
