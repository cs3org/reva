// Copyright 2018-2019 CERN
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

	"github.com/cs3org/reva/pkg/user"
)

func TestImpersonator(t *testing.T) {
	ctx := context.Background()
	i, _ := New(nil)
	ctx, err := i.Authenticate(ctx, "admin", "pwd")
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := user.ContextGetUserID(ctx)
	if !ok {
		t.Fatal("no userid in context")
	}
	if uid.OpaqueId != "admin" {
		t.Errorf("%#v, wanted %#v", uid.OpaqueId, "admin")
	}
	if uid.Idp != "" {
		t.Errorf("%#v, wanted %#v", uid.Idp, "")
	}

	ctx = context.Background()
	ctx, err = i.Authenticate(ctx, "opaqueid@idp", "pwd")
	if err != nil {
		t.Fatal(err)
	}
	uid, ok = user.ContextGetUserID(ctx)
	if !ok {
		t.Fatal("no userid in context")
	}
	if uid.OpaqueId != "opaqueid" {
		t.Errorf("%#v, wanted %#v", uid.OpaqueId, "opaqueid")
	}
	if uid.Idp != "idp" {
		t.Errorf("%#v, wanted %#v", uid.Idp, "idp")
	}
}
