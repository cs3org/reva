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

package invoke

import (
	"context"
	"testing"
)

// TestSet exercises the developer-facing builder: declaring a method, its
// catalog entry, required-arg validation, and routing to the handler.
func TestSet(t *testing.T) {
	set := NewSet()
	set.Add("is_member", "Report whether a user is a member of a group").
		Arg("group", "the group id").
		Arg("user", "the user id").
		Handle(func(_ context.Context, a Args) (Result, error) {
			return Result{"member": a.String("group") == "admins" && a.String("user") == "alice"}, nil
		})

	// Catalog reflects the declaration.
	specs := set.Invocations()
	if len(specs) != 1 || specs[0].Name != "is_member" || specs[0].Kind != KindReadonly {
		t.Fatalf("unexpected catalog: %+v", specs)
	}
	if len(specs[0].Args) != 2 || !specs[0].Args[0].Required {
		t.Fatalf("unexpected args: %+v", specs[0].Args)
	}

	// Routing + typed args.
	res, err := set.Invoke(context.Background(), "is_member", map[string]any{"group": "admins", "user": "alice"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res["member"] != true {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Missing required arg is rejected before the handler runs.
	if _, err := set.Invoke(context.Background(), "is_member", map[string]any{"group": "admins"}); err == nil {
		t.Fatal("expected error for missing required arg")
	}

	// Unknown method.
	if _, err := set.Invoke(context.Background(), "nope", nil); err == nil {
		t.Fatal("expected error for unknown method")
	}
}
