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

package grants

import (
	"reflect"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/gdexlab/go-render/render"
	"github.com/mohae/deepcopy"
)

func TestAddGrant(t *testing.T) {

	testCases := []struct {
		description string
		initial     []*provider.Grant
		newGrant    *provider.Grant
		expected    []*provider.Grant
	}{
		{
			description: "empty-grants-add-positive",
			initial:     []*provider.Grant{},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
				},
				// read permission
				Permissions: &provider.ResourcePermissions{
					Stat:                 true,
					InitiateFileDownload: true,
				},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// read permission
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
			},
		},
		{
			description: "empty-grants-add-negative",
			initial:     []*provider.Grant{},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
				},
				// denial permission
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// read permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "all-positive-add-positive",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					Permissions: &provider.ResourcePermissions{
						UpdateGrant:     true,
						CreateContainer: true,
					},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				Permissions: &provider.ResourcePermissions{
					Stat: true,
				},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					Permissions: &provider.ResourcePermissions{
						UpdateGrant:     true,
						CreateContainer: true,
					},
				},
			},
		},
		{
			description: "all-positive-add-negative",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				// denial permission
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "all-negative-add-positive",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				// read permission
				Permissions: &provider.ResourcePermissions{
					Stat:                 true,
					InitiateFileDownload: true,
				},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					// read permission
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "all-negative-add-negative",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				// denial permission
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "add-positive-not-in-list",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				Permissions: &provider.ResourcePermissions{
					Stat: true,
				},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "add-negative-not-in-list",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
				},
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "9999"}},
					},
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "add-positive-already-in-list",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
				},
				Permissions: &provider.ResourcePermissions{
					CreateContainer: true,
					ListGrants:      true,
				},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						CreateContainer: true,
						ListGrants:      true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
		{
			description: "add-negative-was-positive-in-list",
			initial: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
			newGrant: &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
				},
				// denial permission
				Permissions: &provider.ResourcePermissions{},
			},
			expected: []*provider.Grant{
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1001"}},
					},
					Permissions: &provider.ResourcePermissions{
						Delete:  true,
						GetPath: true,
						Move:    true,
					},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1002"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
				{
					Grantee: &provider.Grantee{
						Type: provider.GranteeType_GRANTEE_TYPE_USER,
						Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: "1000"}},
					},
					// denial permission
					Permissions: &provider.ResourcePermissions{},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {

			grants := deepcopy.Copy(test.initial).([]*provider.Grant)
			AddGrant(&grants, test.newGrant)

			if !reflect.DeepEqual(test.expected, grants) {
				t.Fatalf("lists of grants do not correspond: got=%v expected=%v", render.Render(grants), render.Render(test.expected))
			}

		})
	}

}
