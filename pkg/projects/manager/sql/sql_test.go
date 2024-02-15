// Copyright 2018-2024 CERN
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

package sql_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	projects "github.com/cs3org/reva/pkg/projects/manager/sql"
	"github.com/cs3org/reva/pkg/spaces"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/gdexlab/go-render/render"
)

var (
	dbName        = "reva_tests"
	address       = "localhost"
	port          = 33059
	m             sync.Mutex // for increasing the port
	projectsTable = "projects"
)

func startDatabase(ctx *sql.Context, tables map[string]*memory.Table) (engine *sqle.Engine, p int, cleanup func()) {
	m.Lock()
	defer m.Unlock()

	db := memory.NewDatabase(dbName)
	db.EnablePrimaryKeyIndexes()
	for name, table := range tables {
		db.AddTable(name, table)
	}

	p = port
	config := server.Config{
		Protocol: "tcp",
		Address:  fmt.Sprintf("%s:%d", address, p),
	}
	port++
	engine = sqle.NewDefault(memory.NewMemoryDBProvider(db))
	s, err := server.NewDefaultServer(config, engine)
	if err != nil {
		panic(err)
	}

	go func() {
		if err := s.Start(); err != nil {
			panic(err)
		}
	}()
	cleanup = func() {
		if err := s.Close(); err != nil {
			panic(err)
		}
	}
	return
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func createProjectsTable(ctx *sql.Context, initData []*projects.Project) map[string]*memory.Table {
	tables := make(map[string]*memory.Table)

	// projects table
	tableProjects := memory.NewTable(projectsTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "storage_id", Type: sql.Text, Nullable: false, Source: projectsTable},
		{Name: "path", Type: sql.Text, Nullable: false, Source: projectsTable},
		{Name: "name", Type: sql.Text, Nullable: false, Source: projectsTable, PrimaryKey: true},
		{Name: "owner", Type: sql.Text, Nullable: false, Source: projectsTable},
		{Name: "readers", Type: sql.Text, Nullable: false, Source: projectsTable},
		{Name: "writers", Type: sql.Text, Nullable: false, Source: projectsTable},
		{Name: "admins", Type: sql.Text, Nullable: false, Source: projectsTable},
	}), &memory.ForeignKeyCollection{})

	tables[projectsTable] = tableProjects

	for _, p := range initData {
		must(tableProjects.Insert(ctx, sql.NewRow(p.StorageID, p.Path, p.Name, p.Owner, p.Readers, p.Writers, p.Admins)))
	}

	return tables
}

func TestListProjects(t *testing.T) {
	tests := []struct {
		description string
		projects    []*projects.Project
		user        *userpb.User
		expected    []*provider.StorageSpace
	}{
		{
			description: "empty list",
			projects:    []*projects.Project{},
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    []*provider.StorageSpace{},
		},
		{
			description: "user is owner of the projects",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "owner",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}},
			expected: []*provider.StorageSpace{
				{
					Id: &provider.StorageSpaceId{
						OpaqueId: spaces.EncodeSpaceID("storage_id", "/path/to/project"),
					},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							OpaqueId: "owner",
						},
					},
					Name:      "project",
					SpaceType: spaces.SpaceTypeProject.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path:          "/path/to/project",
						PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
					},
				},
			},
		},
		{
			description: "user part of the readers group",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "unknown",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}, Groups: []string{"project-readers"}},
			expected: []*provider.StorageSpace{
				{
					Id: &provider.StorageSpaceId{
						OpaqueId: spaces.EncodeSpaceID("storage_id", "/path/to/project"),
					},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							OpaqueId: "unknown",
						},
					},
					Name:      "project",
					SpaceType: spaces.SpaceTypeProject.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path:          "/path/to/project",
						PermissionSet: conversions.NewReaderRole().CS3ResourcePermissions(),
					},
				},
			},
		},
		{
			description: "user part of the writers group",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "unknown",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}, Groups: []string{"project-writers"}},
			expected: []*provider.StorageSpace{
				{
					Id: &provider.StorageSpaceId{
						OpaqueId: spaces.EncodeSpaceID("storage_id", "/path/to/project"),
					},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							OpaqueId: "unknown",
						},
					},
					Name:      "project",
					SpaceType: spaces.SpaceTypeProject.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path:          "/path/to/project",
						PermissionSet: conversions.NewEditorRole().CS3ResourcePermissions(),
					},
				},
			},
		},
		{
			description: "user part of the admins group",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "unknown",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}, Groups: []string{"project-admins"}},
			expected: []*provider.StorageSpace{
				{
					Id: &provider.StorageSpaceId{
						OpaqueId: spaces.EncodeSpaceID("storage_id", "/path/to/project"),
					},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							OpaqueId: "unknown",
						},
					},
					Name:      "project",
					SpaceType: spaces.SpaceTypeProject.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path:          "/path/to/project",
						PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
					},
				},
			},
		},
		{
			description: "user part of the admins and readers group",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "unknown",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}, Groups: []string{"project-readers", "project-admins"}},
			expected: []*provider.StorageSpace{
				{
					Id: &provider.StorageSpaceId{
						OpaqueId: spaces.EncodeSpaceID("storage_id", "/path/to/project"),
					},
					Owner: &userpb.User{
						Id: &userpb.UserId{
							OpaqueId: "unknown",
						},
					},
					Name:      "project",
					SpaceType: spaces.SpaceTypeProject.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path:          "/path/to/project",
						PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
					},
				},
			},
		},
		{
			description: "user is neither the owner nor part of the projects' groups",
			projects: []*projects.Project{
				{
					StorageID: "storage_id",
					Path:      "/path/to/project",
					Name:      "project",
					Owner:     "unknown",
					Readers:   "project-readers",
					Writers:   "project-writers",
					Admins:    "project-admins",
				},
			},
			user:     &userpb.User{Id: &userpb.UserId{OpaqueId: "owner", Idp: "idp"}, Groups: []string{"something-readers"}},
			expected: []*provider.StorageSpace{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createProjectsTable(ctx, tt.projects)
			_, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := projects.NewFromConfig(ctx, &projects.Config{
				DBUsername: "root",
				DBPassword: "",
				DBAddress:  fmt.Sprintf("%s:%d", address, port),
				DBName:     dbName,
			})
			if err != nil {
				t.Fatalf("not expected error while creating projects driver: %+v", err)
			}

			got, err := r.ListProjects(context.TODO(), tt.user)
			if err != nil {
				t.Fatalf("not expected error while listing projects: %+v", err)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("projects' list do not match. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		})
	}
}
