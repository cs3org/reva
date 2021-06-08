// Copyright 2018-2021 CERN
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

package eosbinary

import (
	"context"
	"path"
	"reflect"
	"testing"

	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/thanhpk/randstr"
)

const (
	uid     = "0"
	gid     = "0"
	dirTest = "/eos/homecanary/opstest/testacl"
	mgm     = "root://eoshomecanary.cern.ch"
)

func createTempDirectory(ctx context.Context, t *testing.T, client *Client, rootDir string) (string, func()) {
	path := path.Join(rootDir, randstr.String(8))
	err := client.CreateDir(ctx, uid, gid, path)
	if err != nil {
		t.Fatalf("error creating temp folder %s: %v", path, err)
	}
	cleanup := func() {
		err := client.Remove(ctx, uid, gid, path)
		if err != nil {
			t.Fatalf("error deleting folder %s: %v", path, err)
		}
	}
	return path, cleanup
}

func createTempFile(ctx context.Context, t *testing.T, client *Client, dir string) string {
	path := path.Join(dir, randstr.String(8))
	err := client.Touch(ctx, uid, gid, path)
	if err != nil {
		t.Fatalf("error creating new file %s: %v", path, err)
	}
	return path
}

func TestACLOrder(t *testing.T) {

	addACL := func(ctx context.Context, t *testing.T, client *Client, dir string, entry *acl.Entry) {
		err := client.AddACL(ctx, uid, uid, uid, uid, dir, entry)
		if err != nil {
			t.Fatalf("error setting acl %s: %v", entry.CitrineSerialize(), err)
		}
	}

	removeACL := func(ctx context.Context, t *testing.T, client *Client, dir string, entry *acl.Entry) {
		err := client.RemoveACL(ctx, uid, gid, uid, gid, dir, entry)
		if err != nil {
			t.Fatalf("error removing acl: %v", err)
		}
	}

	eosclient := New(&Options{
		URL: mgm,
	})

	testCases := []struct {
		description string
		initial     []*acl.Entry
		action      func(context.Context, *testing.T, *Client, string)
		expected    []*acl.Entry
	}{
		{
			description: "add-acl-from-one",
			initial: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user1",
					Permissions: "rw",
				},
			},
			action: func(ctx context.Context, t *testing.T, client *Client, dir string) {
				addACL(ctx, t, client, dir, &acl.Entry{
					Type:        "u",
					Qualifier:   "user2",
					Permissions: "r",
				})
			},
			expected: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user1",
					Permissions: "rw",
				},
				{
					Type:        "u",
					Qualifier:   "user2",
					Permissions: "r",
				},
			},
		},
		{
			description: "remove-left-acl-from-three",
			initial: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user_left",
					Permissions: "rw",
				},
				{
					Type:        "u",
					Qualifier:   "user_center",
					Permissions: "r",
				},
				{
					Type:        "u",
					Qualifier:   "user_right",
					Permissions: "w",
				},
			},
			action: func(ctx context.Context, t *testing.T, client *Client, dir string) {
				removeACL(ctx, t, client, dir, &acl.Entry{
					Qualifier: "user_left",
				})
			},
			expected: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user_center",
					Permissions: "r",
				},
				{
					Type:        "u",
					Qualifier:   "user_right",
					Permissions: "w",
				},
			},
		},
		{
			description: "remove-center-acl-from-three",
			initial: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user_left",
					Permissions: "rw",
				},
				{
					Type:        "u",
					Qualifier:   "user_center",
					Permissions: "r",
				},
				{
					Type:        "u",
					Qualifier:   "user_right",
					Permissions: "w",
				},
			},
			action: func(ctx context.Context, t *testing.T, client *Client, dir string) {
				removeACL(ctx, t, client, dir, &acl.Entry{
					Qualifier: "user_center",
				})
			},
			expected: []*acl.Entry{
				{
					Type:        "u",
					Qualifier:   "user_left",
					Permissions: "rw",
				},
				{
					Type:        "u",
					Qualifier:   "user_right",
					Permissions: "w",
				},
			},
		},
		{
			description: "remove-right-acl-from-three",
			initial: []*acl.Entry{
				{
					Type:        "user",
					Qualifier:   "user_left",
					Permissions: "rw",
				},
				{
					Type:        "user",
					Qualifier:   "user_center",
					Permissions: "r",
				},
				{
					Type:        "user",
					Qualifier:   "user_right",
					Permissions: "w",
				},
			},
			action: func(ctx context.Context, t *testing.T, client *Client, dir string) {
				removeACL(ctx, t, client, dir, &acl.Entry{
					Qualifier: "user_right",
				})
			},
			expected: []*acl.Entry{
				{
					Type:        "user",
					Qualifier:   "user_left",
					Permissions: "rw",
				},
				{
					Type:        "user",
					Qualifier:   "user_center",
					Permissions: "r",
				},
			},
		},
	}

	for _, test := range testCases {

		t.Run(test.description, func(t *testing.T) {
			ctx := context.TODO()
			dir, cleanup := createTempDirectory(ctx, t, eosclient, dirTest)
			defer cleanup()

			// fill dir with a file
			createTempFile(ctx, t, eosclient, dir)

			// initialize acls
			removeACL(ctx, t, eosclient, dir, &acl.Entry{})

			for _, acl := range test.initial {
				addACL(ctx, t, eosclient, dir, acl)
			}

			test.action(ctx, t, eosclient, dir)

			// verify expected result
			acls, err := eosclient.ListACLs(ctx, uid, gid, dir)
			if err != nil {
				t.Fatalf("error getting acls: %v", err)
			}

			if !reflect.DeepEqual(acls, test.expected) {
				t.Fatalf("acls differ from expected: got=%v expected=%v", acls, test.expected)
			}
		})
	}

}
