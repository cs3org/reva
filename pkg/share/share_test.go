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
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func TestIsCreatedByUser(t *testing.T) {
	user := userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s1 := collaborationv1beta1.Share{
		Owner: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s2 := collaborationv1beta1.Share{
		Creator: &userv1beta1.UserId{
			Idp:      "sampleidp",
			OpaqueId: "user",
		},
	}

	s3 := collaborationv1beta1.Share{
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

	s1 := collaborationv1beta1.Share{
		Grantee: &providerv1beta1.Grantee{
			Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerv1beta1.Grantee_UserId{
				UserId: &userv1beta1.UserId{
					Idp:      "sampleidp",
					OpaqueId: "another",
				},
			},
		},
	}

	s2 := collaborationv1beta1.Share{
		Grantee: &providerv1beta1.Grantee{
			Type: providerv1beta1.GranteeType_GRANTEE_TYPE_GROUP,
			Id: &providerv1beta1.Grantee_GroupId{
				GroupId: &groupv1beta1.GroupId{OpaqueId: "groupid"}},
		},
	}

	if !IsGrantedToUser(&s1, &user) || !IsGrantedToUser(&s2, &user) {
		t.Error("Expected the share to be granted to user")
	}

	s3 := collaborationv1beta1.Share{
		Grantee: &providerv1beta1.Grantee{},
	}

	if IsGrantedToUser(&s3, &user) {
		t.Error("Expecte the share not to be granted to user")
	}
}
