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
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Masterminds/sprig"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"gorm.io/gorm"
)

var _ projects.Admin = (*ProjectsManager)(nil)

var statuses = []projects.ProjectStatus{projects.ProjectStatusCreating, projects.ProjectStatusActive, projects.ProjectStatusArchiving, projects.ProjectStatusArchived}

func parseTemplates(c *Config) (map[string]*template.Template, error) {
	tpls := map[string]*template.Template{}
	for name, text := range map[string]string{
		"path":       c.PathTemplate,
		"storage_id": c.StorageIDTemplate,
		"readers":    c.ReadersGroupTemplate,
		"writers":    c.WritersGroupTemplate,
		"admins":     c.AdminsGroupTemplate,
	} {
		if text == "" {
			continue
		}
		t, err := template.New(name).Funcs(sprig.TxtFuncMap()).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("projects: the %s template: %w", name, err)
		}
		tpls[name] = t
	}
	return tpls, nil
}

func checkStatus(s projects.ProjectStatus) error {
	if !slices.Contains(statuses, s) {
		return errtypes.BadRequest(fmt.Sprintf("status must be creating, active, archiving or archived, not %q", s))
	}
	return nil
}

// ListProjectRecords implements projects.Admin.
func (m *ProjectsManager) ListProjectRecords(ctx context.Context, f projects.Filter) ([]*projects.Record, error) {
	q := m.db.WithContext(ctx).Model(&Project{})
	if f.Status != "" {
		if err := checkStatus(f.Status); err != nil {
			return nil, err
		}
		q = q.Where("status = ?", f.Status)
	}
	if f.Owner != "" {
		q = q.Where("owner = ?", f.Owner)
	}
	if !f.CreatedAfter.IsZero() {
		q = q.Where("created_at >= ?", f.CreatedAfter)
	}
	if !f.CreatedBefore.IsZero() {
		q = q.Where("created_at <= ?", f.CreatedBefore)
	}
	// Asking for the archived projects implies them.
	if !f.IncludeArchived && f.Status != projects.ProjectStatusArchived {
		q = q.Where("archived_at IS NULL")
	}
	var ps []*Project
	if err := q.Order("name").Find(&ps).Error; err != nil {
		return nil, err
	}
	out := make([]*projects.Record, 0, len(ps))
	for _, p := range ps {
		var perms *provider.ResourcePermissions
		if f.Member != nil {
			var ok bool
			if perms, ok = projectBelongsToUser(f.Member, p); !ok {
				continue
			}
		}
		out = append(out, record(p, perms))
	}
	return out, nil
}

// ProjectRecord implements projects.Admin.
func (m *ProjectsManager) ProjectRecord(ctx context.Context, ref projects.Ref) (*projects.Record, error) {
	p, err := m.find(ctx, ref, false)
	if err != nil {
		return nil, err
	}
	return record(p, nil), nil
}

// SetProjectStatus implements projects.Admin. Archiving records when, and
// leaving the archived status frees the time again.
func (m *ProjectsManager) SetProjectStatus(ctx context.Context, ref projects.Ref, status projects.ProjectStatus) (*projects.Record, error) {
	if err := checkStatus(status); err != nil {
		return nil, err
	}
	p, err := m.find(ctx, ref, false)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"status": status}
	switch {
	case status == projects.ProjectStatusArchived && !p.ArchivedAt.Valid:
		updates["archived_at"] = time.Now()
	case status != projects.ProjectStatusArchived && p.ArchivedAt.Valid:
		// Only one project with a name can be live.
		var n int64
		if err := m.db.WithContext(ctx).Unscoped().Model(&Project{}).Where("name = ? AND archived_at IS NULL", p.Name).Count(&n).Error; err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, errtypes.AlreadyExists(fmt.Sprintf("project %s: another project with this name is not archived", p.Name))
		}
		updates["archived_at"] = nil
	}
	if err := m.db.WithContext(ctx).Model(p).Updates(updates).Error; err != nil {
		return nil, err
	}
	m.InvalidateCache()
	return m.reload(ctx, p.ID)
}

// SetProjectOwner implements projects.Admin.
func (m *ProjectsManager) SetProjectOwner(ctx context.Context, ref projects.Ref, owner string) (*projects.Record, error) {
	if owner == "" {
		return nil, errtypes.BadRequest("an owner is required")
	}
	p, err := m.find(ctx, ref, false)
	if err != nil {
		return nil, err
	}
	if err := m.db.WithContext(ctx).Model(p).Update("owner", owner).Error; err != nil {
		return nil, err
	}
	m.InvalidateCache()
	return m.reload(ctx, p.ID)
}

// RegisterProject implements projects.Admin. An empty path, storage id or
// role group comes from its template.
func (m *ProjectsManager) RegisterProject(ctx context.Context, r projects.Registration) (*projects.Record, error) {
	// The name goes into paths and group names.
	if r.Name == "" || r.Name == "." || r.Name == ".." || strings.ContainsRune(r.Name, '/') || strings.ContainsFunc(r.Name, unicode.IsSpace) {
		return nil, errtypes.BadRequest(fmt.Sprintf("%q is not a valid project name", r.Name))
	}
	if r.Owner == "" {
		return nil, errtypes.BadRequest("an owner is required")
	}
	initial, _ := utf8.DecodeRuneInString(r.Name)
	data := struct{ Name, Initial string }{Name: r.Name, Initial: string(initial)}
	for _, v := range []struct {
		name  string
		value *string
	}{{"path", &r.Path}, {"storage_id", &r.StorageID}, {"readers", &r.Readers}, {"writers", &r.Writers}, {"admins", &r.Admins}} {
		if *v.value != "" {
			continue
		}
		t, ok := m.templates[v.name]
		if !ok {
			return nil, errtypes.BadRequest(v.name + " is required: no template is configured for it")
		}
		var b strings.Builder
		if err := t.Execute(&b, data); err != nil {
			return nil, fmt.Errorf("projects: the %s template: %w", v.name, err)
		}
		if *v.value = b.String(); *v.value == "" {
			return nil, errtypes.BadRequest(fmt.Sprintf("the %s template gives an empty value for %s", v.name, r.Name))
		}
	}
	if !path.IsAbs(r.Path) {
		return nil, errtypes.BadRequest(fmt.Sprintf("the path %q is not absolute", r.Path))
	}
	r.Path = path.Clean(r.Path)
	if r.SpaceID == "" {
		r.SpaceID = spaces.EncodeSpaceID(r.Path)
	}
	if r.Status == "" {
		r.Status = projects.ProjectStatusActive
	}
	if err := checkStatus(r.Status); err != nil {
		return nil, err
	}

	// A soft-deleted project keeps its name and space id taken, so that its
	// row can still be restored.
	var n int64
	err := m.db.WithContext(ctx).Unscoped().Model(&Project{}).
		Where("(name = ? AND archived_at IS NULL) OR space_id = ?", r.Name, r.SpaceID).
		Count(&n).Error
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, errtypes.AlreadyExists(fmt.Sprintf("project %s: the name or the space id is taken", r.Name))
	}
	p := &Project{
		Name:        r.Name,
		Owner:       r.Owner,
		Path:        r.Path,
		StorageID:   r.StorageID,
		SpaceID:     r.SpaceID,
		Readers:     r.Readers,
		Writers:     r.Writers,
		Admins:      r.Admins,
		Description: r.Description,
		Status:      r.Status,
	}
	if err := m.db.WithContext(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	m.InvalidateCache()
	return record(p, nil), nil
}

// UnregisterProject implements projects.Admin. With hard, it also finds a
// project that was soft-deleted before.
func (m *ProjectsManager) UnregisterProject(ctx context.Context, ref projects.Ref, hard bool) (*projects.Record, error) {
	p, err := m.find(ctx, ref, hard)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(ctx)
	if hard {
		q = q.Unscoped()
	}
	if err := q.Delete(p).Error; err != nil {
		return nil, err
	}
	m.InvalidateCache()
	return record(p, nil), nil
}

func (m *ProjectsManager) find(ctx context.Context, ref projects.Ref, unscoped bool) (*Project, error) {
	q := m.db.WithContext(ctx)
	if unscoped {
		q = q.Unscoped()
	}
	switch {
	case ref.SpaceID != "" && ref.Name != "":
		return nil, errtypes.BadRequest("give a space id or a name, not both")
	case ref.SpaceID != "":
		// A CS3 space id is <storage id>$<space id>.
		id := ref.SpaceID
		if _, s, ok := strings.Cut(id, "$"); ok {
			id = s
		}
		if id == "" {
			return nil, errtypes.BadRequest(fmt.Sprintf("%q is not a space id", ref.SpaceID))
		}
		q = q.Where("space_id = ?", id)
	case ref.Name != "":
		q = q.Where("name = ?", ref.Name).Order("archived_at IS NULL DESC").Order("id DESC")
	default:
		return nil, errtypes.BadRequest("a space id or a name is required")
	}
	var p Project
	if err := q.First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errtypes.NotFound("project " + cmp.Or(ref.Name, ref.SpaceID))
		}
		return nil, err
	}
	return &p, nil
}

func (m *ProjectsManager) reload(ctx context.Context, id uint) (*projects.Record, error) {
	var p Project
	if err := m.db.WithContext(ctx).Where("id = ?", id).First(&p).Error; err != nil {
		return nil, err
	}
	return record(&p, nil), nil
}

// record gives p as an operator sees it. Extra holds the columns that CS3
// cannot carry, and only the ones that are set.
func record(p *Project, perms *provider.ResourcePermissions) *projects.Record {
	extra := map[string]string{}
	for k, v := range map[string]string{
		"capabilities":              p.Capabilities,
		"resource_admins":           p.ResourceAdmins,
		"user_provided_description": p.UserProvidedDescription,
		"service_account":           p.ServiceAccount,
		"comments":                  p.Comments,
		"snow_ticket":               p.SnowTicket,
		"backup_name":               p.BackupName,
	} {
		if v != "" {
			extra[k] = v
		}
	}
	if p.InitialCapacityBytes > 0 {
		extra["initial_capacity_bytes"] = strconv.FormatUint(p.InitialCapacityBytes, 10)
	}
	extra["created_at"] = p.CreatedAt.UTC().Format(time.RFC3339)
	extra["updated_at"] = p.UpdatedAt.UTC().Format(time.RFC3339)
	if p.ArchivedAt.Valid {
		extra["archived_at"] = p.ArchivedAt.V.UTC().Format(time.RFC3339)
	}
	if p.DeletedAt.Valid {
		extra["deleted_at"] = p.DeletedAt.Time.UTC().Format(time.RFC3339)
	}
	return &projects.Record{Space: projectToStorageSpace(p, perms), Status: p.Status, Extra: extra}
}
