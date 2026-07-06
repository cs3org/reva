// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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

package gateway

import (
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

func userGrantee(id string) *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: id}},
	}
}

func managerGrant(grantee *provider.Grantee, exp *typesv1beta1.Timestamp) *provider.Grant {
	return &provider.Grant{
		Grantee:     grantee,
		Permissions: &provider.ResourcePermissions{RemoveGrant: true},
		Expiration:  exp,
	}
}

func viewerGrant(grantee *provider.Grantee, exp *typesv1beta1.Timestamp) *provider.Grant {
	return &provider.Grant{
		Grantee:     grantee,
		Permissions: &provider.ResourcePermissions{RemoveGrant: false},
		Expiration:  exp,
	}
}

// TestMayRemoveLastPermanentManager covers the guard that prevents orphaning a
// space: a manager grant that gains an expiration must be treated the same as a
// manager that loses the role, because an expiring manager is not a permanent
// manager.
func TestMayRemoveLastPermanentManager(t *testing.T) {
	alice := userGrantee("alice")
	exp := &typesv1beta1.Timestamp{Seconds: 1}

	tests := []struct {
		name  string
		grant *provider.Grant
		want  bool
	}{
		{"permanent manager, unchanged", managerGrant(alice, nil), false},
		{"manager gains an expiration", managerGrant(alice, exp), true},
		{"manager role dropped", viewerGrant(alice, nil), true},
		{"non-manager with expiration", viewerGrant(alice, exp), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mayRemoveLastPermanentManager(tt.grant); got != tt.want {
				t.Errorf("mayRemoveLastPermanentManager() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsSpaceManagerRemaining documents the invariant the guard relies on: only
// another permanent (non-expiring), non-self manager keeps a space managed.
func TestIsSpaceManagerRemaining(t *testing.T) {
	alice := userGrantee("alice")
	bob := userGrantee("bob")
	exp := &typesv1beta1.Timestamp{Seconds: 1}

	tests := []struct {
		name     string
		grants   []*provider.Grant
		updating *provider.Grantee
		want     bool
	}{
		{"another permanent manager remains", []*provider.Grant{managerGrant(alice, nil), managerGrant(bob, nil)}, alice, true},
		{"only the updated grantee is manager", []*provider.Grant{managerGrant(alice, nil)}, alice, false},
		{"the other manager is expiring", []*provider.Grant{managerGrant(alice, nil), managerGrant(bob, exp)}, alice, false},
		{"the other grantee is only a viewer", []*provider.Grant{managerGrant(alice, nil), viewerGrant(bob, nil)}, alice, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSpaceManagerRemaining(tt.grants, tt.updating); got != tt.want {
				t.Errorf("isSpaceManagerRemaining() = %v, want %v", got, tt.want)
			}
		})
	}
}
