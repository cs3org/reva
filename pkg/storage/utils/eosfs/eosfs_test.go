// Copyright 2018-2023 CERN
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

//go:build eos
// +build eos

package eosfs

import (
	"context"
	"os/exec"
	"path"
	"reflect"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/gdexlab/go-render/render"
	"github.com/thanhpk/randstr"
)

const (
	uid     string = "0"
	gid     string = "0"
	rootDir string = "/eos/homecanary/opstest/testacl/grants"
)

func createTempDirectory(ctx context.Context, t *testing.T, eos *eosfs, rootDir string) (string, func()) {
	path := path.Join(rootDir, randstr.String(8))
	err := eos.c.CreateDir(ctx, uid, gid, path)
	if err != nil {
		t.Fatalf("error creating temp folder %s: %v", path, err)
	}
	cleanup := func() {
		err := eos.c.Remove(ctx, uid, gid, path)
		if err != nil {
			t.Fatalf("error deleting folder %s: %v", path, err)
		}
	}
	return path, cleanup
}

func createTempFile(ctx context.Context, t *testing.T, eos *eosfs, dir string) (string, func()) {
	path := path.Join(dir, randstr.String(8))
	err := eos.c.Touch(ctx, uid, gid, path)
	if err != nil {
		t.Fatalf("error creating new file %s: %v", path, err)
	}
	cleanup := func() {
		err := eos.c.Remove(ctx, uid, gid, path)
		if err != nil {
			t.Fatalf("error deleting folder %s: %v", path, err)
		}
	}
	return path, cleanup
}

// return true if the command exist
func commandExists(path string) bool {
	_, err := exec.LookPath(path)
	return err == nil
}

func TestAddGrant(t *testing.T) {

	if !commandExists("/usr/bin/eos") {
		t.Skip("/usr/bin/eos does not exist")
	}

	fs, err := NewEOSFS(&Config{
		MasterURL:           "root://eoshomecanary.cern.ch",
		UseGRPC:             false,
		ForceSingleUserMode: true,
	})

	if err != nil {
		t.Fatal("error creating a new EOS client:", err)
	}

	eos, ok := fs.(*eosfs)
	if !ok {
		t.Fatal("error creating a new EOS client:", err)
	}

	testCases := []struct {
		description string
		initial     string
		grant       *provider.Grant
		expected    []*provider.Grant
	}{
		{
			description: "all-positive",
			initial:     "u:1:r,u:2:w,u:3:rw",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
				Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true, CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
				},
			},
		},
		{
			description: "all-negative",
			initial:     "u:1:!r!w!x!m!u!d,u:2:!r!w!x!m!u!d,u:3:!r!w!x!m!u!d",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "user-not-in-grant-list-add-positive",
			initial:     "u:1:rw,u:2:r,u:3:!r!w!x!m!u!d",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
				Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true, CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "user-not-in-grant-list-add-negative",
			initial:     "u:1:rw,u:2:r,u:3:!r!w!x!m!u!d",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true, CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9"}}},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "user-in-grant-list-add-positive",
			initial:     "u:1:r,u:2:r,u:3:!r!w!x!m!u!d",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
				Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true, CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true, CreateContainer: true, InitiateFileUpload: true, Delete: true, Move: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "user-in-grant-list-add-negative",
			initial:     "u:1:r,u:2:r,u:3:!r!w!x!m!u!d",
			grant: &provider.Grant{
				Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "2"}}},
					Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "3"}}},
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee:     &provider.Grantee{Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1"}}},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
	}

	for _, test := range testCases {

		t.Run(test.description, func(t *testing.T) {
			ctx := appctx.ContextSetUser(context.TODO(), &userv1beta1.User{
				UidNumber: 138406,
				GidNumber: 2763,
			})

			// test grants for a folder
			dir, cleanupDir := createTempDirectory(ctx, t, eos, rootDir)
			defer cleanupDir()

			// set initial acls
			err := eos.c.SetAttr(ctx, uid, gid, &eosclient.Attribute{Type: SystemAttr, Key: "acl", Val: test.initial}, true, dir)
			if err != nil {
				t.Fatal("error setting initial attributes:", err)
			}

			dirRef := &provider.Reference{Path: dir}

			// set new grant
			err = eos.AddGrant(ctx, dirRef, test.grant)
			if err != nil {
				t.Fatal("error adding grant:", err)
			}

			// check that the new grants list corresponds to expected result
			grants, err := eos.ListGrants(ctx, dirRef)
			if err != nil {
				t.Fatal("error getting grants:", err)
			}

			if !reflect.DeepEqual(grants, test.expected) {
				t.Fatalf("grants do not correspond in folder %s: got=%v expected=%v", dir, render.AsCode(grants), render.AsCode(test.expected))
			}

			// test grants for a file
			file, cleanupFile := createTempFile(ctx, t, eos, rootDir)
			defer cleanupFile()

			// set initial acls
			err = eos.c.SetAttr(ctx, uid, gid, &eosclient.Attribute{Type: UserAttr, Key: "acl", Val: test.initial}, true, dir)
			if err != nil {
				t.Fatal("error setting initial attributes:", err)
			}

			fileRef := &provider.Reference{Path: file}

			// set new grant
			err = eos.AddGrant(ctx, fileRef, test.grant)
			if err != nil {
				t.Fatal("error adding grant:", err)
			}

			// check that the new grants list corresponds to expected result
			grants, err = eos.ListGrants(ctx, fileRef)
			if err != nil {
				t.Fatal("error getting grants:", err)
			}

			if !reflect.DeepEqual(grants, test.expected) {
				t.Fatalf("grants do not correspond in file %s: got=%v expected=%v", file, render.AsCode(grants), render.AsCode(test.expected))
			}
		})

	}

}
