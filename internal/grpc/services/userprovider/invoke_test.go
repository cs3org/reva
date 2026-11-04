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

package userprovider

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	_ "github.com/cs3org/reva/v3/pkg/user/manager/demo"
)

func newDemo(t *testing.T) invoke.Invokable {
	t.Helper()
	svc, err := New(context.Background(), map[string]any{"driver": "demo"})
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := svc.(invoke.Invokable)
	if !ok {
		t.Fatal("the userprovider is not invokable")
	}
	return inv
}

func TestInvocationsAreSharedReads(t *testing.T) {
	for _, spec := range newDemo(t).Invocations() {
		if spec.Kind != invoke.KindReadonly || spec.Scope != invoke.ScopeShared {
			t.Errorf("%s: kind %q scope %q, want a shared read", spec.Name, spec.Kind, spec.Scope)
		}
	}
}

func TestGet(t *testing.T) {
	s := newDemo(t)
	for _, args := range []map[string]any{
		{"user": "einstein"},
		{"claim": "mail", "value": "einstein@example.org"},
		{"claim": "uid", "value": "123"},
	} {
		res, err := s.Invoke(context.Background(), "get", args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		want := invoke.Result{
			"id":            "4c510ada-c86b-4815-8820-42cdf82c3d51",
			"idp":           "http://localhost:9998",
			"type":          "primary",
			"username":      "einstein",
			"display_name":  "Albert Einstein",
			"mail":          "einstein@example.org",
			"mail_verified": false,
			"uid_number":    int64(123),
			"gid_number":    int64(987),
			"status":        "active",
		}
		for k, v := range want {
			if res[k] != v {
				t.Errorf("%v: %s is %v, want %v", args, k, res[k], v)
			}
		}
	}
}

func TestGetUnknownUserIsNotFound(t *testing.T) {
	_, err := newDemo(t).Invoke(context.Background(), "get", map[string]any{"user": "nobody"})
	if !errors.As(err, new(errtypes.NotFound)) {
		t.Fatalf("got %v, want a not found error", err)
	}
}

func TestGetNeedsOneSelector(t *testing.T) {
	s := newDemo(t)
	for _, args := range []map[string]any{
		{},
		{"user": "einstein", "claim": "mail", "value": "einstein@example.org"},
		{"claim": "mail"},
	} {
		_, err := s.Invoke(context.Background(), "get", args)
		if !errors.As(err, new(errtypes.BadRequest)) {
			t.Errorf("%v: got %v, want a bad request", args, err)
		}
	}
}

func TestGroupsAreSorted(t *testing.T) {
	res, err := newDemo(t).Invoke(context.Background(), "groups", map[string]any{"user": "marie"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"physics-lovers", "polonium-lovers", "radium-lovers"}
	if got := res["groups"].([]string); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFindSortsByUsername(t *testing.T) {
	res, err := newDemo(t).Invoke(context.Background(), "find", map[string]any{"query": "example.org"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range res["users"].([]map[string]any) {
		got = append(got, u["username"].(string))
	}
	if want := []string{"einstein", "marie", "richard"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
