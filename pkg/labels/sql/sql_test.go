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

package sql

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

func TestListLabelsForResources(t *testing.T) {
	m, err := New(map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "labels.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	as := func(user string) context.Context {
		return appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: user}})
	}

	a := &provider.ResourceId{StorageId: "eos", OpaqueId: "100"}
	b := &provider.ResourceId{StorageId: "eos", OpaqueId: "200"}
	// Same inode as a on another instance.
	c := &provider.ResourceId{StorageId: "other", OpaqueId: "100"}
	unlabeled := &provider.ResourceId{StorageId: "eos", OpaqueId: "300"}

	for _, set := range []struct {
		user  string
		label string
		id    *provider.ResourceId
	}{
		{"me", "favorite", a},
		{"me", "work", a},
		{"me", "favorite", b},
		{"me", "favorite", c},
		{"someone", "favorite", unlabeled},
	} {
		if err := m.SetLabel(as(set.user), set.label, set.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.UnsetLabel(as("me"), "favorite", b); err != nil {
		t.Fatal(err)
	}

	// The space ID is ignored, because label rows have none.
	requested := &provider.ResourceId{StorageId: "eos", SpaceId: "space", OpaqueId: "100"}
	got, err := m.ListLabelsForResources(as("me"), []*provider.ResourceId{requested, b, unlabeled})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got labels for %d resources, want 1: %v", len(got), got)
	}
	labels := got["eos!100"]
	slices.Sort(labels)
	if !slices.Equal(labels, []string{"favorite", "work"}) {
		t.Errorf("labels of eos!100 = %v, want [favorite work]", labels)
	}

	got, err = m.ListLabelsForResources(as("me"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("no resources: got %v, want no labels", got)
	}
}
