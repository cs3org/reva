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

package spacesregistry

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/projects"
	projectsql "github.com/cs3org/reva/v3/pkg/projects/manager/sql"
)

// The spaces registry is an invoke.Invokable: through the Admin API an
// operator can list every project of the catalogue (the CS3 surface only
// lists the caller's), read one, change its status or owner, record a new one
// or delete a record, and flush the per-process project cache. Provisioning
// the space on the storage is not done here; the record is the catalogue's
// view of a project that exists (or is being created) on EOS.

// adminCatalogue is what the invocations need from the projects catalogue
// beyond the CS3 Catalogue. The SQL driver implements it; any other driver
// makes the invocations answer that they are unsupported.
type adminCatalogue interface {
	ListProjects(ctx context.Context, f projectsql.ProjectFilter) ([]*projectsql.Project, error)
	GetProject(ctx context.Context, name string) (*projectsql.Project, error)
	GetAnyProject(ctx context.Context, name string) (*projectsql.Project, error)
	UpdateProjectStatus(ctx context.Context, name string, status projects.ProjectStatus) error
	UpdateProjectOwner(ctx context.Context, name, owner string) error
	CreateProject(ctx context.Context, p *projectsql.Project) error
	DeleteProject(ctx context.Context, name string, hard bool) error
	InvalidateCache()
}

func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("list", "List the projects of the catalogue, every owner included.").
		OptArg("status", "only projects in this status: creating, active, archiving or archived").
		OptArg("owner", "only projects owned by this user").
		OptArg("created_after", "only projects created at or after this date or RFC 3339 time").
		OptArg("created_before", "only projects created at or before this date or RFC 3339 time").
		OptArg("include_archived", "include archived projects (default false)").
		Handle(s.invokeList)
	s.Set.Add("get", "Show one project of the catalogue, archived or not.").
		Arg("name", "the project name").
		Handle(s.invokeGet)
	s.Set.Add("set_status", "Set the status of a project; archived also records when. Address one instance by node id, then flush_cache everywhere.").
		Mutating().
		Arg("name", "the project name").
		Arg("status", "creating, active, archiving or archived").
		Handle(s.invokeSetStatus)
	s.Set.Add("set_owner", "Change the owner of a project. Address one instance by node id, then flush_cache everywhere.").
		Mutating().
		Arg("name", "the project name").
		Arg("owner", "the new owner, usually the service account of the project").
		Handle(s.invokeSetOwner)
	s.Set.Add("create", "Record a project in the catalogue; the space itself must exist on the storage. Address one instance by node id, then flush_cache everywhere.").
		Dangerous().
		Arg("name", "the project name").
		Arg("owner", "the owner, usually the service account of the project").
		OptArg("path", "the path of the space (default "+projectsql.ProjectPathPrefix+"<initial>/<name>)").
		OptArg("storage_id", "the storage serving the space (default: that of the projects under the same initial)").
		OptArg("space_id", "the space id (default: derived from the path)").
		OptArg("status", "creating, active, archiving or archived (default active)").
		OptArg("readers", "the readers e-group (default cernbox-project-<name>-readers)").
		OptArg("writers", "the writers e-group (default cernbox-project-<name>-writers)").
		OptArg("admins", "the admins e-group (default cernbox-project-<name>-admins)").
		OptArg("resource_admins", "the resource admins e-group").
		OptArg("description", "the description shown to users").
		OptArg("service_account", "the service account linked to the project").
		OptArg("snow_ticket", "the ticket that requested the project").
		OptArg("comments", "comments for support").
		OptArg("capabilities", "comma-separated capabilities").
		Handle(s.invokeCreate)
	s.Set.Add("delete", "Delete a project record: soft by default, which keeps the name taken; hard=true removes the row. Address one instance by node id, then flush_cache everywhere.").
		Dangerous().
		Arg("name", "the project name").
		OptArg("hard", "remove the row for good").
		Handle(s.invokeDelete)
	s.Set.Add("flush_cache", "Drop this instance's cached project list, so the next listing reads the database. Run it on every instance after a mutation.").
		Mutating().
		Handle(s.invokeFlushCache)
}

func (s *service) admin() (adminCatalogue, error) {
	c, ok := s.projects.(adminCatalogue)
	if !ok {
		return nil, fmt.Errorf("spacesregistry: the %s driver does not support admin invocations", s.c.Driver)
	}
	return c, nil
}

func (s *service) invokeList(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	f := projectsql.ProjectFilter{Owner: a.String("owner"), IncludeArchived: a.Bool("include_archived")}
	if a.Has("status") {
		if f.Status, err = parseStatus(a.String("status")); err != nil {
			return nil, err
		}
	}
	if f.CreatedAfter, _, err = admin.ParseTime(a.String("created_after")); err != nil {
		return nil, err
	}
	if f.CreatedBefore, _, err = admin.ParseTime(a.String("created_before")); err != nil {
		return nil, err
	}
	ps, err := c.ListProjects(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]invoke.Result, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectResult(p))
	}
	return invoke.Result{"projects": out, "count": len(out)}, nil
}

func (s *service) invokeGet(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	p, err := c.GetAnyProject(ctx, a.String("name"))
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", a.String("name"), err)
	}
	return projectResult(p), nil
}

func (s *service) invokeSetStatus(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	name := a.String("name")
	status, err := parseStatus(a.String("status"))
	if err != nil {
		return nil, err
	}
	err = c.UpdateProjectStatus(ctx, name, status)
	audit(ctx, "set_status", name, err, map[string]string{"status": string(status)})
	if err != nil {
		return nil, err
	}
	return s.result(ctx, c, name)
}

func (s *service) invokeSetOwner(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	name, owner := a.String("name"), a.String("owner")
	if _, err := admin.Lookup(ctx, owner); err != nil {
		return nil, err
	}
	p, err := c.GetProject(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", name, err)
	}
	err = c.UpdateProjectOwner(ctx, name, owner)
	audit(ctx, "set_owner", name, err, map[string]string{"owner": owner, "from": p.Owner})
	if err != nil {
		return nil, err
	}
	return s.result(ctx, c, name)
}

func (s *service) invokeCreate(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(a.String("name"))
	if name == "" || strings.ContainsAny(name, "/ ") {
		return nil, fmt.Errorf("%q is not a valid project name", name)
	}
	if _, err := admin.Lookup(ctx, a.String("owner")); err != nil {
		return nil, err
	}
	p := &projectsql.Project{
		Name:           name,
		Owner:          a.String("owner"),
		Path:           a.String("path"),
		StorageID:      a.String("storage_id"),
		SpaceID:        a.String("space_id"),
		Readers:        a.String("readers"),
		Writers:        a.String("writers"),
		Admins:         a.String("admins"),
		ResourceAdmins: a.String("resource_admins"),
		Description:    a.String("description"),
		ServiceAccount: a.String("service_account"),
		SnowTicket:     a.String("snow_ticket"),
		Comments:       a.String("comments"),
		Capabilities:   a.String("capabilities"),
	}
	if a.Has("status") {
		if p.Status, err = parseStatus(a.String("status")); err != nil {
			return nil, err
		}
	}
	err = c.CreateProject(ctx, p)
	audit(ctx, "create", name, err, map[string]string{"owner": p.Owner, "path": p.Path, "storage_id": p.StorageID})
	if err != nil {
		return nil, err
	}
	r := projectResult(p)
	r["created"] = true
	return r, nil
}

func (s *service) invokeDelete(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	name, hard := a.String("name"), a.Bool("hard")
	err = c.DeleteProject(ctx, name, hard)
	audit(ctx, "delete", name, err, map[string]string{"hard": strconv.FormatBool(hard)})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"deleted": name, "hard": hard}, nil
}

func (s *service) invokeFlushCache(ctx context.Context, _ invoke.Args) (invoke.Result, error) {
	c, err := s.admin()
	if err != nil {
		return nil, err
	}
	c.InvalidateCache()
	return invoke.Result{"flushed": true}, nil
}

func (s *service) result(ctx context.Context, c adminCatalogue, name string) (invoke.Result, error) {
	p, err := c.GetAnyProject(ctx, name)
	if err != nil {
		return nil, err
	}
	return projectResult(p), nil
}

func parseStatus(v string) (projects.ProjectStatus, error) {
	st := projects.ProjectStatus(strings.ToLower(strings.TrimSpace(v)))
	switch st {
	case projects.ProjectStatusCreating, projects.ProjectStatusActive, projects.ProjectStatusArchiving, projects.ProjectStatusArchived:
		return st, nil
	}
	return "", fmt.Errorf("status must be creating, active, archiving or archived, not %q", v)
}

func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "spacesregistry." + action,
		Actor:   admin.Actor(ctx),
		Target:  target,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

// projectResult renders a project record with stable snake_case keys.
func projectResult(p *projectsql.Project) invoke.Result {
	r := invoke.Result{
		"name":                      p.Name,
		"path":                      p.Path,
		"owner":                     p.Owner,
		"status":                    string(p.Status),
		"readers":                   p.Readers,
		"writers":                   p.Writers,
		"admins":                    p.Admins,
		"resource_admins":           p.ResourceAdmins,
		"description":               p.Description,
		"readme_path":               p.ReadmePath,
		"thumbnail_path":            p.ThumbnailPath,
		"capabilities":              p.Capabilities,
		"service_account":           p.ServiceAccount,
		"snow_ticket":               p.SnowTicket,
		"comments":                  p.Comments,
		"user_provided_description": p.UserProvidedDescription,
		"backup_name":               p.BackupName,
		"initial_capacity_bytes":    p.InitialCapacityBytes,
		"space_id":                  p.SpaceID,
		"storage_id":                p.StorageID,
		"created_at":                admin.FormatTime(p.CreatedAt),
		"updated_at":                admin.FormatTime(p.UpdatedAt),
	}
	if p.ArchivedAt.Valid {
		r["archived_at"] = admin.FormatTime(p.ArchivedAt.V)
	}
	if p.DeletedAt.Valid {
		r["deleted_at"] = admin.FormatTime(p.DeletedAt.Time)
	}
	return r
}
