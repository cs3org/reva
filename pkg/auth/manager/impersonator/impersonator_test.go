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

package impersonator

import (
	"context"
	"testing"
)

func TestImpersonator(t *testing.T) {
	ctx := context.Background()
	i, _ := New(nil)
	u, err := i.Authenticate(ctx, "admin", "pwd")
	if err != nil {
		t.Fatal(err)
	}

	if u.Id.OpaqueId != "admin" {
		t.Errorf("%#v, wanted %#v", u.Id.OpaqueId, "admin")
	}
	if u.Id.Idp != "" {
		t.Errorf("%#v, wanted %#v", u.Id.Idp, "")
	}

	ctx = context.Background()
	u, err = i.Authenticate(ctx, "opaqueid@idp", "pwd")
	if err != nil {
		t.Fatal(err)
	}
	if u.Id.OpaqueId != "opaqueid" {
		t.Errorf("%#v, wanted %#v", u.Id.OpaqueId, "opaqueid")
	}
	if u.Id.Idp != "idp" {
		t.Errorf("%#v, wanted %#v", u.Id.Idp, "idp")
	}
}
