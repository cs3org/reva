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

package gateway

import (
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

func TestUserTypeLabel(t *testing.T) {
	for typ, want := range map[userpb.UserType]string{
		userpb.UserType_USER_TYPE_PRIMARY:     "primary",
		userpb.UserType_USER_TYPE_SECONDARY:   "secondary",
		userpb.UserType_USER_TYPE_SERVICE:     "service",
		userpb.UserType_USER_TYPE_LIGHTWEIGHT: "lightweight",
		userpb.UserType_USER_TYPE_FEDERATED:   "federated",
	} {
		if got := userTypeLabel(typ); got != want {
			t.Errorf("userTypeLabel(%v) = %q, want %q", typ, got, want)
		}
	}
}
