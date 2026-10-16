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

package sql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// The methods below are the administrative surface of the projects catalogue,
// used by the spacesregistry's admin invocations. They are not part of the
// CS3-defined Catalogue.

// ProjectPathPrefix is where project spaces live.
const ProjectPathPrefix = "/eos/project/"

// ProjectFilter selects projects for an administrative listing. A zero field
// does not filter.
type ProjectFilter struct {
	Status        projects.ProjectStatus
	Owner         string
	CreatedAfter  time.Time
	CreatedBefore time.Time
	// IncludeArchived also returns projects with archived_at set.
	IncludeArchived bool
}

// ProjectPath is the path of a project named name: /eos/project/<initial>/<name>.
func ProjectPath(name string) string {
	return ProjectPathPrefix + strings.ToLower(name[:1]) + "/" + name
}

// ListProjects lists the projects matching f, by name.
func (m *ProjectsManager) ListProjects(_ context.Context, f ProjectFilter) ([]*Project, error) {
	query := m.db.Model(&Project{})
	if f.Status != "" {
		query = query.Where("status = ?", f.Status)
	}
	if f.Owner != "" {
		query = query.Where("owner = ?", f.Owner)
	}
	if !f.CreatedAfter.IsZero() {
		query = query.Where("created_at >= ?", f.CreatedAfter)
	}
	if !f.CreatedBefore.IsZero() {
		query = query.Where("created_at <= ?", f.CreatedBefore)
	}
	if !f.IncludeArchived {
		query = query.Where("archived_at IS NULL")
	}
	var fetched []*Project
	res := query.Order("name").Find(&fetched)
	if res.Error != nil {
		return nil, res.Error
	}
	return fetched, nil
}

// GetAnyProject returns a project by name whether it is archived or not,
// unlike GetProject, which only knows live projects.
func (m *ProjectsManager) GetAnyProject(_ context.Context, name string) (*Project, error) {
	var p Project
	res := m.db.Where("name = ?", name).Order("archived_at IS NULL DESC, id DESC").First(&p)
	if res.Error != nil {
		return nil, res.Error
	}
	return &p, nil
}

// UpdateProjectOwner changes the owner of a project.
func (m *ProjectsManager) UpdateProjectOwner(_ context.Context, name, owner string) error {
	if owner == "" {
		return fmt.Errorf("an owner is required")
	}
	res := m.db.Model(&Project{}).Where("name = ?", name).Where("archived_at IS NULL").Update("owner", owner)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("no project found with name %s", name)
	}
	m.InvalidateCache()
	return nil
}

// CreateProject records a project. The path, the space id, the storage id and
// the access groups are derived from the name when they are not given; the
// storage id is copied from the projects already under the same initial, as
// nothing else in the catalogue knows which storage serves a letter.
func (m *ProjectsManager) CreateProject(_ context.Context, p *Project) error {
	if p == nil || p.Name == "" {
		return fmt.Errorf("a name is required")
	}
	if p.Owner == "" {
		return fmt.Errorf("an owner is required")
	}
	if p.Path == "" {
		p.Path = ProjectPath(p.Name)
	}
	p.Path = strings.TrimSuffix(p.Path, "/")
	if p.SpaceID == "" {
		p.SpaceID = spaces.EncodeSpaceID(p.Path)
	}
	if p.StorageID == "" {
		storageID, err := m.storageIDFor(p.Path)
		if err != nil {
			return err
		}
		p.StorageID = storageID
	}
	if p.Status == "" {
		p.Status = projects.ProjectStatusActive
	}
	if p.Readers == "" {
		p.Readers = "cernbox-project-" + p.Name + "-readers"
	}
	if p.Writers == "" {
		p.Writers = "cernbox-project-" + p.Name + "-writers"
	}
	if p.Admins == "" {
		p.Admins = "cernbox-project-" + p.Name + "-admins"
	}
	if res := m.db.Create(p); res.Error != nil {
		return res.Error
	}
	m.InvalidateCache()
	return nil
}

// storageIDFor is the storage id of the projects under the same initial as
// path, or an error asking for an explicit one when there is none.
func (m *ProjectsManager) storageIDFor(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, ProjectPathPrefix)
	if !ok || len(rest) < 2 {
		return "", fmt.Errorf("%q is not a project path, pass storage_id", path)
	}
	prefix := ProjectPathPrefix + rest[:1] + "/"
	var sibling Project
	res := m.db.Unscoped().Model(&Project{}).Where("path LIKE ?", prefix+"%").Where("storage_id <> ''").Order("id desc").Limit(1).Find(&sibling)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", fmt.Errorf("no project under %s to derive the storage id from, pass storage_id", prefix)
	}
	return sibling.StorageID, nil
}

// DeleteProject deletes a project record: soft by default, which keeps the
// name taken until the row is hard-deleted; permanently with hard.
func (m *ProjectsManager) DeleteProject(_ context.Context, name string, hard bool) error {
	query := m.db.Where("name = ?", name)
	if hard {
		query = query.Unscoped()
	}
	res := query.Delete(&Project{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("no project found with name %s", name)
	}
	m.InvalidateCache()
	return nil
}

// InvalidateCache drops the cached project list of this process, so that the
// next listing reads the database. Every mutation calls it; other processes
// keep their copy until it expires or they are asked to flush.
func (m *ProjectsManager) InvalidateCache() {
	_ = m.cache.Remove(cacheKey)
}
