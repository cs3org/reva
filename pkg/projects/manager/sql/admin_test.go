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
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

func newAdminManager(t *testing.T, conf map[string]any) *ProjectsManager {
	t.Helper()
	c := map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "projects.db")}
	maps.Copy(c, conf)
	m, err := New(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return m.(*ProjectsManager)
}

var cernTemplates = map[string]any{
	"path_template":          "/eos/project/{{ .Initial }}/{{ .Name }}",
	"storage_id_template":    "newproject-{{ .Initial }}",
	"readers_group_template": "cernbox-project-{{ .Name }}-readers",
	"writers_group_template": "cernbox-project-{{ .Name }}-writers",
	"admins_group_template":  "cernbox-project-{{ .Name }}-admins",
}

func isErr[T any](err error) bool {
	var e T
	return errors.As(err, &e)
}

func TestRegisterProjectTemplates(t *testing.T) {
	ctx := context.Background()
	m := newAdminManager(t, cernTemplates)
	r, err := m.RegisterProject(ctx, projects.Registration{Name: "cernbox", Owner: "cboxsvc"})
	if err != nil {
		t.Fatal(err)
	}
	sp := r.Space
	if sp.GetRootInfo().GetPath() != "/eos/project/c/cernbox" || sp.GetId().GetOpaqueId() != "newproject-c$"+spaces.EncodeSpaceID("/eos/project/c/cernbox") || r.Status != projects.ProjectStatusActive {
		t.Fatalf("derived values: %v", r)
	}
	groups := map[string]string{}
	for _, role := range sp.GetRoles() {
		groups[role.GetRoleName()] = role.GetRecipients()[0].GetGroupId().GetOpaqueId()
	}
	if groups["reader"] != "cernbox-project-cernbox-readers" || groups["writer"] != "cernbox-project-cernbox-writers" || groups["admin"] != "cernbox-project-cernbox-admins" {
		t.Fatalf("groups: %v", groups)
	}
	if r.Extra["created_at"] == "" {
		t.Fatalf("extra: %v", r.Extra)
	}

	// A given value wins over the template.
	r, err = m.RegisterProject(ctx, projects.Registration{Name: "atlas", Owner: "atlassvc", Path: "/eos/project/a/atlas/", StorageID: "eosproject-a", Admins: "atlas-admins"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Space.GetRootInfo().GetPath() != "/eos/project/a/atlas" || r.Space.GetRoles()[2].GetRecipients()[0].GetGroupId().GetOpaqueId() != "atlas-admins" {
		t.Fatalf("given values: %v", r.Space)
	}

	for _, reg := range []projects.Registration{
		{Name: "cernbox", Owner: "x"},                // the name is taken
		{Name: "other", Owner: "x", SpaceID: "Y"},    // fine
		{Name: "again", Owner: "x", SpaceID: "Y"},    // the space id is taken
		{Name: "a/b", Owner: "x"},                    // not a name
		{Name: "..", Owner: "x"},                     // not a name
		{Name: "a b", Owner: "x"},                    // not a name
		{Name: "nobody"},                             // no owner
		{Name: "rel", Owner: "x", Path: "eos/x/rel"}, // not absolute
		{Name: "bad", Owner: "x", Status: "deleted"}, // not a status
	} {
		_, err := m.RegisterProject(ctx, reg)
		if (err == nil) != (reg.Name == "other") {
			t.Errorf("register %+v: %v", reg, err)
		}
	}
}

func TestRegisterProjectWithoutTemplates(t *testing.T) {
	ctx := context.Background()
	m := newAdminManager(t, nil)
	_, err := m.RegisterProject(ctx, projects.Registration{Name: "cernbox", Owner: "cboxsvc", Path: "/p/cernbox", StorageID: "s", Readers: "r", Writers: "w"})
	if !isErr[errtypes.IsBadRequest](err) {
		t.Fatalf("the admins group is required without a template: %v", err)
	}
	if _, err := m.RegisterProject(ctx, projects.Registration{Name: "cernbox", Owner: "cboxsvc", Path: "/p/cernbox", StorageID: "s", Readers: "r", Writers: "w", Admins: "a"}); err != nil {
		t.Fatal(err)
	}

	if _, err := New(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "x.db"), "path_template": "{{ .Name"}); err == nil {
		t.Fatal("a template that does not parse must fail at start")
	}
}

func TestProjectRecordRefs(t *testing.T) {
	ctx := context.Background()
	m := newAdminManager(t, cernTemplates)
	old, err := m.RegisterProject(ctx, projects.Registration{Name: "cms", Owner: "a", Path: "/eos/project/c/cms-old"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetProjectStatus(ctx, projects.Ref{Name: "cms"}, projects.ProjectStatusArchived); err != nil {
		t.Fatal(err)
	}
	// An archived project leaves its name free.
	live, err := m.RegisterProject(ctx, projects.Registration{Name: "cms", Owner: "b"})
	if err != nil {
		t.Fatal(err)
	}

	if r, err := m.ProjectRecord(ctx, projects.Ref{Name: "cms"}); err != nil || r.Space.GetOwner().GetId().GetOpaqueId() != "b" {
		t.Fatalf("by name, the live project: %v %v", r, err)
	}
	if r, err := m.ProjectRecord(ctx, projects.Ref{SpaceID: old.Space.GetId().GetOpaqueId()}); err != nil || r.Status != projects.ProjectStatusArchived || r.Extra["archived_at"] == "" {
		t.Fatalf("by CS3 space id, the archived project: %v %v", r, err)
	}
	if _, err := m.ProjectRecord(ctx, projects.Ref{SpaceID: spaces.EncodeSpaceID("/eos/project/c/cms")}); err != nil {
		t.Fatalf("by space id: %v", err)
	}
	// Un-archiving the old project would give two live projects named cms.
	if _, err := m.SetProjectStatus(ctx, projects.Ref{SpaceID: old.Space.GetId().GetOpaqueId()}, projects.ProjectStatusActive); !isErr[errtypes.IsAlreadyExists](err) {
		t.Fatalf("un-archive with a live namesake: %v", err)
	}
	if _, err := m.UnregisterProject(ctx, projects.Ref{SpaceID: live.Space.GetId().GetOpaqueId()}, true); err != nil {
		t.Fatal(err)
	}
	r, err := m.SetProjectStatus(ctx, projects.Ref{Name: "cms"}, projects.ProjectStatusActive)
	if err != nil || r.Status != projects.ProjectStatusActive || r.Extra["archived_at"] != "" {
		t.Fatalf("un-archive: %v %v", r, err)
	}

	for _, ref := range []projects.Ref{{}, {Name: "cms", SpaceID: "x"}, {SpaceID: "storage$"}} {
		if _, err := m.ProjectRecord(ctx, ref); !isErr[errtypes.IsBadRequest](err) {
			t.Errorf("ref %+v: %v", ref, err)
		}
	}
	for _, ref := range []projects.Ref{{Name: "nope"}, {SpaceID: "nope"}} {
		if _, err := m.ProjectRecord(ctx, ref); !isErr[errtypes.IsNotFound](err) {
			t.Errorf("ref %+v must not be found: %v", ref, err)
		}
	}
}

func TestListProjectRecords(t *testing.T) {
	ctx := context.Background()
	m := newAdminManager(t, cernTemplates)
	for _, reg := range []projects.Registration{
		{Name: "atlas", Owner: "atlassvc"},
		{Name: "cernbox", Owner: "cboxsvc", Status: projects.ProjectStatusCreating},
		{Name: "cms", Owner: "cmssvc"},
	} {
		if _, err := m.RegisterProject(ctx, reg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.SetProjectStatus(ctx, projects.Ref{Name: "cms"}, projects.ProjectStatusArchived); err != nil {
		t.Fatal(err)
	}
	names := func(f projects.Filter) []string {
		t.Helper()
		rs, err := m.ListProjectRecords(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rs {
			out = append(out, r.Space.GetName())
		}
		return out
	}
	for _, c := range []struct {
		f    projects.Filter
		want []string
	}{
		{projects.Filter{}, []string{"atlas", "cernbox"}},
		{projects.Filter{IncludeArchived: true}, []string{"atlas", "cernbox", "cms"}},
		{projects.Filter{Status: projects.ProjectStatusArchived}, []string{"cms"}},
		{projects.Filter{Status: projects.ProjectStatusCreating}, []string{"cernbox"}},
		{projects.Filter{Owner: "atlassvc"}, []string{"atlas"}},
		{projects.Filter{Member: &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Groups: []string{"cernbox-project-atlas-writers", "cernbox-project-cms-readers"}}, IncludeArchived: true}, []string{"atlas", "cms"}},
		{projects.Filter{Member: &userpb.User{Id: &userpb.UserId{OpaqueId: "cboxsvc"}}}, []string{"cernbox"}},
	} {
		if got := names(c.f); !slices.Equal(got, c.want) {
			t.Errorf("%+v: got %v, want %v", c.f, got, c.want)
		}
	}
	rs, _ := m.ListProjectRecords(ctx, projects.Filter{Member: &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Groups: []string{"cernbox-project-atlas-writers"}}})
	if len(rs) != 1 || !rs[0].Space.GetRootInfo().GetPermissionSet().GetInitiateFileUpload() || rs[0].Space.GetRootInfo().GetPermissionSet().GetAddGrant() {
		t.Fatalf("a member gets the permissions of the role: %v", rs)
	}
	if _, err := m.ListProjectRecords(ctx, projects.Filter{Status: "bogus"}); !isErr[errtypes.IsBadRequest](err) {
		t.Fatalf("unknown status: %v", err)
	}
}

func TestUnregisterProject(t *testing.T) {
	ctx := context.Background()
	m := newAdminManager(t, cernTemplates)
	if _, err := m.RegisterProject(ctx, projects.Registration{Name: "atlas", Owner: "atlassvc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UnregisterProject(ctx, projects.Ref{Name: "atlas"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ProjectRecord(ctx, projects.Ref{Name: "atlas"}); !isErr[errtypes.IsNotFound](err) {
		t.Fatalf("a deleted project must not be found: %v", err)
	}
	if _, err := m.RegisterProject(ctx, projects.Registration{Name: "atlas", Owner: "atlassvc", Path: "/elsewhere/atlas"}); !isErr[errtypes.IsAlreadyExists](err) {
		t.Fatalf("a soft-deleted project keeps its name: %v", err)
	}
	if _, err := m.UnregisterProject(ctx, projects.Ref{Name: "atlas"}, false); !isErr[errtypes.IsNotFound](err) {
		t.Fatalf("soft delete twice: %v", err)
	}
	if _, err := m.UnregisterProject(ctx, projects.Ref{Name: "atlas"}, true); err != nil {
		t.Fatalf("a hard delete finds a soft-deleted project: %v", err)
	}
	if _, err := m.RegisterProject(ctx, projects.Registration{Name: "atlas", Owner: "atlassvc"}); err != nil {
		t.Fatal(err)
	}
}

// The catalogue lists projects to users from a cache, which a mutation of
// this process must drop.
func TestMutationsInvalidateTheCache(t *testing.T) {
	m := newAdminManager(t, cernTemplates)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "atlassvc"}})
	listed := func() []*provider.StorageSpace {
		t.Helper()
		res, err := m.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{}, projects.ProjectStatusActive)
		if err != nil {
			t.Fatal(err)
		}
		return res.GetStorageSpaces()
	}
	if len(listed()) != 0 {
		t.Fatal("no project yet")
	}
	if _, err := m.RegisterProject(ctx, projects.Registration{Name: "atlas", Owner: "atlassvc"}); err != nil {
		t.Fatal(err)
	}
	if len(listed()) != 1 {
		t.Fatal("register must drop the cache")
	}
	if _, err := m.SetProjectOwner(ctx, projects.Ref{Name: "atlas"}, "other"); err != nil {
		t.Fatal(err)
	}
	if len(listed()) != 0 {
		t.Fatal("set owner must drop the cache")
	}
}
