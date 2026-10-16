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

package dynamic

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	_ "github.com/mattn/go-sqlite3"
)

func newRegistry(t *testing.T) (*dynamic, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "routing.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE routing (path TEXT PRIMARY KEY, mount_id TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for _, r := range [][2]string{{"/eos/user/a", "eoshome-a"}, {"/eos/project/c", "eosproject-c"}, {"/eos/user/z", "nowhere"}} {
		if _, err := db.Exec("INSERT INTO routing (path, mount_id) VALUES (?, ?)", r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	prev := openDB
	openDB = func(string) (*sql.DB, error) { return db, nil }
	t.Cleanup(func() { openDB = prev })
	reg, err := New(context.Background(), map[string]any{
		"rules":     map[string]any{"eoshome-a": "host:1", "eosproject-c": "host:2", "eoshome-b": "host:3"},
		"home_path": "/eos/user/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	return reg.(*dynamic), db
}

func resolve(t *testing.T, d *dynamic, p string) string {
	t.Helper()
	ps, err := d.FindProviders(context.Background(), &provider.Reference{Path: p})
	if err != nil {
		return ""
	}
	if len(ps) != 1 {
		t.Fatalf("resolve %s: %v", p, ps)
	}
	return ps[0].Address
}

func TestRoutesAreLoadedAndAdministered(t *testing.T) {
	d, db := newRegistry(t)
	if a := resolve(t, d, "/eos/user/a/alice"); a != "host:1" {
		t.Fatalf("home route: %q", a)
	}
	// a route to a mount that is not configured is skipped, not fatal
	if a := resolve(t, d, "/eos/user/z/zed"); a != "" {
		t.Fatalf("unconfigured mount resolved to %q", a)
	}
	routes, err := d.ListRoutes(context.Background())
	if err != nil || len(routes) != 3 || routes[0].Path != "/eos/project/c" {
		t.Fatalf("list: %v %v", routes, err)
	}

	// adding a route takes effect at once
	if err := d.AddRoute(context.Background(), "/eos/user/b", "eoshome-b"); err != nil {
		t.Fatal(err)
	}
	if a := resolve(t, d, "/eos/user/b/bob"); a != "host:3" {
		t.Fatalf("added route: %q", a)
	}
	// updating one too
	if err := d.AddRoute(context.Background(), "/eos/user/b", "eoshome-a"); err != nil {
		t.Fatal(err)
	}
	if a := resolve(t, d, "/eos/user/b/bob"); a != "host:1" {
		t.Fatalf("updated route: %q", a)
	}
	for _, c := range [][2]string{{"/eos/user/q", "unknown-mount"}, {"relative", "eoshome-a"}, {"", "eoshome-a"}} {
		if err := d.AddRoute(context.Background(), c[0], c[1]); err == nil {
			t.Errorf("add %v must fail", c)
		}
	}

	// removing
	if err := d.RemoveRoute(context.Background(), "/eos/user/b"); err != nil {
		t.Fatal(err)
	}
	if a := resolve(t, d, "/eos/user/b/bob"); a != "" {
		t.Fatalf("removed route still resolves to %q", a)
	}
	if err := d.RemoveRoute(context.Background(), "/eos/user/b"); err == nil {
		t.Fatal("removing an unknown route must be reported")
	}

	// a change made behind the registry's back shows after a reload
	if _, err := db.Exec("INSERT INTO routing (path, mount_id) VALUES (?, ?)", "/eos/user/c", "eoshome-a"); err != nil {
		t.Fatal(err)
	}
	if a := resolve(t, d, "/eos/user/c/carol"); a != "" {
		t.Fatalf("route visible before reload: %q", a)
	}
	if err := d.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a := resolve(t, d, "/eos/user/c/carol"); a != "host:1" {
		t.Fatalf("route after reload: %q", a)
	}
	// by resource id, through the rules
	ps, err := d.FindProviders(context.Background(), &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "eosproject-c", OpaqueId: "1"}})
	if err != nil || len(ps) != 1 || ps[0].Address != "host:2" {
		t.Fatalf("by id: %v %v", ps, err)
	}
}
