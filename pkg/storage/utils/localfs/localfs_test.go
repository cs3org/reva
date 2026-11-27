// Copyright 2018-2024 CERN
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

package localfs

import (
"context"
"testing"

userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
"github.com/cs3org/reva/v3/pkg/appctx"
"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
)

// TestHomeTemplateMemoization pins the per-user memoization of GetHome and
// getVirtualHome: values must be correct, distinct users must not share an
// entry, and a user whose template inputs change must be re-expanded rather
// than served a stale layout.
func TestHomeTemplateMemoization(t *testing.T) {
config := &Config{
Root:                t.TempDir(),
UserLayout:          "{{.Username}}",
VirtualHomeTemplate: "/home/{{.Username}}",
}
config.ApplyDefaults()
fs := &localfs{conf: config}

alice := &userpb.User{
Id:       &userpb.UserId{OpaqueId: "alice", Idp: "https://idp.example.test"},
Username: "alice",
}
bob := &userpb.User{
Id:       &userpb.UserId{OpaqueId: "bob", Idp: "https://idp.example.test"},
Username: "bob",
}
ctxAlice := appctx.ContextSetUser(context.Background(), alice)
ctxBob := appctx.ContextSetUser(context.Background(), bob)

for i := 0; i < 3; i++ {
if got, err := fs.GetHome(ctxAlice); err != nil || got != "alice" {
t.Fatalf("GetHome(alice) #%d = %q, %v; want alice", i, got, err)
}
if got, err := fs.getVirtualHome(ctxAlice); err != nil || got != "/home/alice" {
t.Fatalf("getVirtualHome(alice) #%d = %q, %v; want /home/alice", i, got, err)
}
}
if got, err := fs.GetHome(ctxBob); err != nil || got != "bob" {
t.Fatalf("GetHome(bob) = %q, %v; want bob (cross-user bleed?)", got, err)
}
if got, err := fs.getVirtualHome(ctxBob); err != nil || got != "/home/bob" {
t.Fatalf("getVirtualHome(bob) = %q, %v; want /home/bob (cross-user bleed?)", got, err)
}
if got, _ := fs.GetHome(ctxAlice); got != "alice" {
t.Fatalf("GetHome(alice) after bob = %q; want alice", got)
}

if _, ok := fs.homeCache.Load(userCacheKey(alice)); !ok {
t.Errorf("homeCache has no entry for alice")
}
if _, ok := fs.vhomeCache.Load(userCacheKey(alice)); !ok {
t.Errorf("vhomeCache has no entry for alice")
}

// Same Id, changed Username: the key must change so the new layout is used.
renamed := &userpb.User{
Id:       &userpb.UserId{OpaqueId: "alice", Idp: "https://idp.example.test"},
Username: "alice2",
}
ctxRenamed := appctx.ContextSetUser(context.Background(), renamed)
if got, err := fs.GetHome(ctxRenamed); err != nil || got != "alice2" {
t.Fatalf("GetHome(renamed) = %q, %v; want alice2 (stale memo?)", got, err)
}
}

// BenchmarkGetHomeMemoized vs BenchmarkWithUserDirect shows the cost the memo
// removes: templates.WithUser re-parses the template and runs it through
// text/template on every call.
func BenchmarkGetHomeMemoized(b *testing.B) {
config := &Config{Root: b.TempDir(), UserLayout: "{{.Username}}"}
config.ApplyDefaults()
fs := &localfs{conf: config}
u := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "https://idp.example.test"}, Username: "einstein"}
ctx := appctx.ContextSetUser(context.Background(), u)
fs.GetHome(ctx)
b.ResetTimer()
for i := 0; i < b.N; i++ {
fs.GetHome(ctx)
}
}

func BenchmarkWithUserDirect(b *testing.B) {
u := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "https://idp.example.test"}, Username: "einstein"}
b.ResetTimer()
for i := 0; i < b.N; i++ {
templates.WithUser(u, "{{.Username}}")
}
}
