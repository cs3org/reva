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
	"reflect"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	projects_catalogue "github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// You can use testing.T, if you want to test the code without benchmarking
func setupSuite(tb testing.TB) (projects_catalogue.Catalogue, error, func(tb testing.TB) error) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]any{
		"engine":  "sqlite",
		"db_name": dbName,
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", "/path/to/project"),
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
					PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
				},
			},
		},
		{
			description: "user part of the readers group",
			projects: []*Project{
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", "/path/to/project"),
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
						PermissionSet: conversions.NewViewerRole().CS3ResourcePermissions(),
					},
					PermissionSet: conversions.NewViewerRole().CS3ResourcePermissions(),
				},
			},
		},
		{
			description: "user part of the writers group",
			projects: []*Project{
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", "/path/to/project"),
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
					PermissionSet: conversions.NewEditorRole().CS3ResourcePermissions(),
				},
			},
		},
		{
			description: "user part of the admins group",
			projects: []*Project{
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", "/path/to/project"),
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
					PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
				},
			},
		},
		{
			description: "user part of the admins and readers group",
			projects: []*Project{
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
						OpaqueId: spaces.EncodeStorageSpaceID("storage_id", "/path/to/project"),
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
					PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
				},
			},
		},
		{
			description: "user is neither the owner nor part of the projects' groups",
			projects: []*Project{
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
			ctx := context.Background()
			catalogue, err, teardown := setupSuite(t)

			if err != nil {
				t.Error(err)
			}

			if err != nil {
				t.Fatalf("not expected error while creating projects driver: %+v", err)
			}

			catmgr := catalogue.(*ProjectsManager)
			for _, proj := range tt.projects {
				catmgr.db.Create(&proj)
			}

			ctx = appctx.ContextSetUser(ctx, tt.user)
			got, err := catalogue.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{})
			if err != nil {
				t.Fatalf("not expected error while listing projects: %+v", err)
			}

			if !reflect.DeepEqual(got.StorageSpaces, tt.expected) {
				t.Fatalf("projects' list do not match. got=%+v expected=%+v", got.StorageSpaces, tt.expected)
			}

			err = teardown(t)
			if err != nil {
				t.Fatalf("failed to teardown test suite: %+v", err)
			}
		})
	}
}
