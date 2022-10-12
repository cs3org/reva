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

package share

import (
	"testing"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func TestIsCreatedByUser(t *testing.T) {
	user := userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s1 := collaboration.Share{
		Owner: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s2 := collaboration.Share{
		Creator: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s3 := collaboration.Share{
		Owner: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
		Creator: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	if !IsCreatedByUser(&s1, &user) || !IsCreatedByUser(&s2, &user) || !IsCreatedByUser(&s3, &user) {
		t.Error("Expected share to be created by user")
	}

	anotherUser := userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "another",
		},
	}

	if IsCreatedByUser(&s1, &anotherUser) {
		t.Error("Expected share not to be created by user")
	}
}

func TestIsGrantedToUser(t *testing.T) {
	user := userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "another",
		},
		Groups: []string{"groupid"},
	}

	s1 := collaboration.Share{
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userv1beta1.UserId{
					Idp:      "sampleidp",
					OpaqueId: "another",
				},
			},
		},
	}

	s2 := collaboration.Share{
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id: &provider.Grantee_GroupId{
				GroupId: &groupv1beta1.GroupId{OpaqueId: "groupid"}},
		},
	}

	if !IsGrantedToUser(&s1, &user) || !IsGrantedToUser(&s2, &user) {
		t.Error("Expected the share to be granted to user")
	}

	s3 := collaboration.Share{
		Grantee: &provider.Grantee{},
	}

	if IsGrantedToUser(&s3, &user) {
		t.Error("Expecte the share not to be granted to user")
	}
}

func TestMatchesFilter(t *testing.T) {
	id := &provider.ResourceId{StorageId: "storage", OpaqueId: "opaque"}
	share := &collaboration.Share{
		ResourceId: id,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
		},
		Permissions: &collaboration.SharePermissions{Permissions: &provider.ResourcePermissions{}},
	}

	if !MatchesFilter(share, NoState, ResourceIDFilter(id)) {
		t.Errorf("Expected share to pass the id filter. Share: %v", share)
	}
	if MatchesFilter(share, NoState, GroupGranteeFilter()) {
		t.Errorf("Expected share to not pass the grantee type filter. Share: %v", share)
	}
	if MatchesFilter(share, NoState, &collaboration.Filter{Type: collaboration.Filter_TYPE_EXCLUDE_DENIALS}) {
		t.Errorf("Expected share to not pass the exclude denials filter. Share: %v", share)
	}
	if MatchesFilter(share, NoState, &collaboration.Filter{Type: collaboration.Filter_TYPE_INVALID}) {
		t.Errorf("Expected share to not pass an invalid filter. Share: %v", share)
	}
}

func TestMatchesAnyFilter(t *testing.T) {
	id := &provider.ResourceId{StorageId: "storage", OpaqueId: "opaque"}
	share := &collaboration.Share{
		ResourceId: id,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
		},
		Permissions: &collaboration.SharePermissions{Permissions: &provider.ResourcePermissions{}},
	}

	f1 := []*collaboration.Filter{UserGranteeFilter(), GroupGranteeFilter()}
	if !MatchesAnyFilter(share, NoState, f1) {
		t.Errorf("Expected share to match any of the given filters. Share: %v, Filters: %v", share, f1)
	}

	f2 := []*collaboration.Filter{ResourceIDFilter(&provider.ResourceId{StorageId: "something", OpaqueId: "different"}), GroupGranteeFilter()}
	if MatchesAnyFilter(share, NoState, f2) {
		t.Errorf("Expected share to not match any of the given filters. Share: %v, Filters: %v", share, f2)
	}
}

func TestMatchesFilters(t *testing.T) {
	id := &provider.ResourceId{StorageId: "storage", OpaqueId: "opaque"}
	share := &collaboration.Share{
		ResourceId: id,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
		},
		Permissions: &collaboration.SharePermissions{Permissions: &provider.ResourcePermissions{}},
	}

	f1 := []*collaboration.Filter{ResourceIDFilter(id), GroupGranteeFilter()}
	if MatchesFilters(share, f1) {
		t.Errorf("Expected share to not match the filters. Share %v, Filters %v", share, f1)
	}

	f2 := []*collaboration.Filter{ResourceIDFilter(id), UserGranteeFilter(), GroupGranteeFilter()}
	if !MatchesFilters(share, f2) {
		t.Errorf("Expected share to match the filters. Share %v, Filters %v", share, f2)
	}
}

func TestGroupFiltersByType(t *testing.T) {
	id := &provider.ResourceId{StorageId: "storage", OpaqueId: "opaque"}
	filters := []*collaboration.Filter{UserGranteeFilter(), GroupGranteeFilter(), ResourceIDFilter(id)}

	grouped := GroupFiltersByType(filters)

	for fType, f := range grouped {
		switch fType {
		case collaboration.Filter_TYPE_GRANTEE_TYPE:
			if len(f) != 2 {
				t.Errorf("Expected 2 grantee type filters got %d", len(f))
			}
			for i := range f {
				if f[i].Type != fType {
					t.Errorf("Filter %v doesn't belong to this type %v", f[i], t)
				}
			}
		case collaboration.Filter_TYPE_RESOURCE_ID:
			if len(f) != 1 {
				t.Errorf("Expected 1 resource id filter got %d", len(f))
			}
			if f[0].Type != fType {
				t.Errorf("Filter %v doesn't belong to this type %v", f[0], t)
			}
		}
	}
}
