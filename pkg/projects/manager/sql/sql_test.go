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

package sql

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	projects_catalogue "github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// You can use testing.T, if you want to test the code without benchmarking
func setupSuite(tb testing.TB) (projects_catalogue.Catalogue, error, func(tb testing.TB) error) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbName,
	}
	mgr, err := New(ctx, cfg)
	if err != nil {
		return nil, err, nil
	}

	// Return a function to teardown the test
	return mgr, nil, func(tb testing.TB) error {
		log.Println("teardown suite")
		return os.Remove(dbName)
	}
}

func TestListProjects(t *testing.T) {

	spaceID := spaces.EncodeSpaceID("/path/to/project")

	// Every project in the test cases below uses these e-groups
	roles := []*provider.SpaceRole{{
		RoleName:      "reader",
		PermissionSet: permissions.NewViewerRole().CS3ResourcePermissions(),
		Recipients: []*provider.Grantee{
			{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id: &provider.Grantee_GroupId{
					GroupId: &groupv1beta1.GroupId{
						OpaqueId: "project-readers",
					},
				},
			},
		},
	}, {
		RoleName:      "writer",
		PermissionSet: permissions.NewEditorRole().CS3ResourcePermissions(),
		Recipients: []*provider.Grantee{
			{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id: &provider.Grantee_GroupId{
					GroupId: &groupv1beta1.GroupId{
						OpaqueId: "project-writers",
					},
				},
			},
		},
	}, {
		RoleName:      "admin",
		PermissionSet: permissions.NewManagerRole().CS3ResourcePermissions(),
		Recipients: []*provider.Grantee{
			{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id: &provider.Grantee_GroupId{
					GroupId: &groupv1beta1.GroupId{
						OpaqueId: "project-admins",
					},
				},
			},
		},
	}}

	tests := []struct {
		description string
		projects    []*Project
		user        *userpb.User
		expected    []*provider.StorageSpace
	}{
		{
			description: "empty list",
			projects:    []*Project{},
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    []*provider.StorageSpace{},
		},
		{
			description: "user is owner of the projects",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", spaceID),
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
						PermissionSet: permissions.NewManagerRole().CS3ResourcePermissions(),
					},
					Roles: roles,
				},
			},
		},
		{
			description: "user part of the readers group",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", spaceID),
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
						PermissionSet: permissions.NewViewerRole().CS3ResourcePermissions(),
					},
					Roles: roles,
				},
			},
		},
		{
			description: "user part of the writers group",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", spaceID),
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
						PermissionSet: permissions.NewEditorRole().CS3ResourcePermissions(),
					},
					Roles: roles,
				},
			},
		},
		{
			description: "user part of the admins group",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", spaceID),
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
						PermissionSet: permissions.NewManagerRole().CS3ResourcePermissions(),
					},
					Roles: roles,
				},
			},
		},
		{
			description: "user part of the admins and readers group",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", spaceID),
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
						PermissionSet: permissions.NewManagerRole().CS3ResourcePermissions(),
					},
					Roles: roles,
				},
			},
		},
		{
			description: "user is neither the owner nor part of the projects' groups",
			projects: []*Project{
				{
					StorageID: "storage_id",
					SpaceID:   spaceID,
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
			ctx := context.Background()
			catalogue, err, teardown := setupSuite(t)

			if err != nil {
				t.Error(err)
			}

			if err != nil {
				t.Fatalf("not expected error while creating projects driver: %+v", err)
			}

			defer func() {
				if err := teardown(t); err != nil {
					t.Fatalf("failed to teardown test suite: %+v", err)
				}
			}()

			catmgr := catalogue.(*ProjectsManager)
			for _, proj := range tt.projects {
				catmgr.db.Create(&proj)
			}

			ctx = appctx.ContextSetUser(ctx, tt.user)
			got, err := catalogue.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{}, projects_catalogue.ProjectStatusActive)
			if err != nil {
				t.Fatalf("not expected error while listing projects: %+v", err)
			}

			if !reflect.DeepEqual(got.StorageSpaces, tt.expected) {
				t.Fatalf("projects' list do not match. got=%+v expected=%+v", got.StorageSpaces, tt.expected)
			}
		})
	}
}

func TestSpaceByIDIgnoresMembership(t *testing.T) {
	catalogue, err, teardown := setupSuite(t)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown(t)
	mgr := catalogue.(*ProjectsManager)
	spaceID := spaces.EncodeSpaceID("/eos/project/c/cernbox")
	mgr.db.Create(&Project{StorageID: "eosproject", SpaceID: spaceID, Path: "/eos/project/c/cernbox", Name: "cernbox", Owner: "owner", Admins: "cernbox-admins"})

	space, err := mgr.SpaceByID(context.Background(), spaces.EncodeStorageSpaceID("eosproject", spaceID))
	if err != nil {
		t.Fatal(err)
	}
	if space.GetName() != "cernbox" || space.GetRoles()[2].GetRoleName() != "admin" || space.GetRoles()[2].GetRecipients()[0].GetGroupId().GetOpaqueId() != "cernbox-admins" {
		t.Fatalf("space %+v", space)
	}
}

// Only the owner and the admins of a project may change it. For anyone
// else, the project is not found and stays as it was.
func TestUpdateStorageSpaceNeedsAManager(t *testing.T) {
	mgr, err := New(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "projects.db")})
	if err != nil {
		t.Fatal(err)
	}
	m := mgr.(*ProjectsManager)
	p := &Project{Name: "atlas", Owner: "atlassvc", SpaceID: "atlas", StorageID: "s", Path: "/p/atlas", Admins: "cernbox-project-atlas-admins", Writers: "cernbox-project-atlas-writers", Description: "before"}
	if err := m.db.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	id := projectToStorageSpace(p, nil).GetId()
	update := func(u *userpb.User, name, desc string) *provider.UpdateStorageSpaceResponse {
		t.Helper()
		res, err := m.UpdateStorageSpace(appctx.ContextSetUser(context.Background(), u), &provider.UpdateStorageSpaceRequest{
			StorageSpace: &provider.StorageSpace{Id: id, Name: name},
			Field:        &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Description{Description: desc}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	description := func() string {
		p, err := m.GetProject(context.Background(), "atlas")
		if err != nil {
			t.Fatal(err)
		}
		return p.Description
	}

	writer := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Groups: []string{"cernbox-project-atlas-writers"}}
	stranger := &userpb.User{Id: &userpb.UserId{OpaqueId: "mallory"}}
	for _, u := range []*userpb.User{writer, stranger} {
		if res := update(u, "atlas", "changed"); res.GetStatus().GetCode() != rpc.Code_CODE_NOT_FOUND {
			t.Fatalf("%s: %v", u.GetId().GetOpaqueId(), res.GetStatus())
		}
	}
	if res := update(stranger, "nope", "changed"); res.GetStatus().GetCode() != rpc.Code_CODE_NOT_FOUND {
		t.Fatalf("a missing project: %v", res.GetStatus())
	}
	if d := description(); d != "before" {
		t.Fatalf("a refused update changed the description to %q", d)
	}

	admin := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}, Groups: []string{"cernbox-project-atlas-admins"}}
	if res := update(admin, "atlas", "by an admin"); res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetStorageSpace().GetDescription() != "by an admin" {
		t.Fatalf("admin: %v", res)
	}
	owner := &userpb.User{Id: &userpb.UserId{OpaqueId: "atlassvc"}}
	if res := update(owner, "atlas", "by the owner"); res.GetStatus().GetCode() != rpc.Code_CODE_OK || description() != "by the owner" {
		t.Fatalf("owner: %v", res)
	}
}
