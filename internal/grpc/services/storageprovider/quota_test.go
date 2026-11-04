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

package storageprovider

import (
	"context"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/storage"
)

type quotaFS struct {
	storage.FS
	quotas map[string]*storage.SpaceQuota
	refs   []storage.SpaceQuotaRef
}

func (f *quotaFS) GetSpaceQuota(_ context.Context, ref storage.SpaceQuotaRef) (*storage.SpaceQuota, error) {
	f.refs = append(f.refs, ref)
	if q, ok := f.quotas[ref.Root]; ok {
		return q, nil
	}
	return &storage.SpaceQuota{Root: ref.Root}, nil
}

func (f *quotaFS) SetSpaceQuota(_ context.Context, ref storage.SpaceQuotaRef, maxBytes, maxFiles uint64) (*storage.SpaceQuota, error) {
	f.quotas[ref.Root] = &storage.SpaceQuota{Root: ref.Root, MaxBytes: maxBytes, MaxFiles: maxFiles, PhysicalMaxBytes: 2 * maxBytes, Form: "project"}
	return f.quotas[ref.Root], nil
}

func (f *quotaFS) ListSpaceQuotas(context.Context) ([]*storage.SpaceQuota, error) {
	return []*storage.SpaceQuota{{Root: "/e/einstein", MaxBytes: 10}, {Root: "/b/bob", MaxBytes: 1000}}, nil
}

func newQuotaService(fs storage.FS, mountPath string, spaceDepth int) *service {
	s := &service{
		Set:       invoke.NewSet(),
		conf:      &config{Driver: "eos", SpaceDepth: spaceDepth},
		storage:   fs,
		mountPath: mountPath,
		mountID:   "eoshome-i01",
	}
	s.addQuotaInvocations()
	return s
}

func TestQuotaInvocations(t *testing.T) {
	ctx := context.Background()
	fs := &quotaFS{quotas: map[string]*storage.SpaceQuota{
		"/e/einstein": {Root: "/e/einstein", MaxBytes: 10, MaxFiles: 100, Form: "project"},
	}}
	s := newQuotaService(fs, "/eos/user", 4)

	if m := s.RegistryMetadata(); m["mount_id"] != "eoshome-i01" || m["mount_path"] != "/eos/user" {
		t.Fatalf("metadata: %v", m)
	}

	owner := admin.UserArg(&userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"})
	res, err := s.Invoke(ctx, "quota_get", map[string]any{"root": "/eos/user/e/einstein", "space_type": "personal", "owner": owner})
	if err != nil || res["root"] != "/eos/user/e/einstein" || res["max_bytes"] != uint64(10) || res["form"] != "project" || res["owner"].(map[string]any)["opaqueId"] != "einstein" {
		t.Fatalf("quota_get: %v %v", res, err)
	}
	if ref := fs.refs[0]; ref.Root != "/e/einstein" || ref.Type != "personal" || ref.Owner.GetOpaqueId() != "einstein" || ref.Owner.GetIdp() != "cernbox" {
		t.Fatalf("ref: %+v", ref)
	}

	for _, root := range []string{"/eos/user", "/eos/user/e", "/eos/user/e/einstein/docs", "/eos/users/e/einstein", "/eos/project/c/cernbox", "e/einstein"} {
		if _, err := s.Invoke(ctx, "quota_set", map[string]any{"root": root, "space_type": "personal", "max_bytes": "1"}); err == nil {
			t.Errorf("root %q must be refused", root)
		}
	}
	if _, err := s.Invoke(ctx, "quota_set", map[string]any{"root": "/eos/user/e/einstein", "space_type": "personal"}); err == nil {
		t.Error("quota_set without a value must be refused")
	}

	res, err = s.Invoke(ctx, "quota_set", map[string]any{"root": "/eos/user/e/einstein", "space_type": "personal", "max_bytes": "2000"})
	if err != nil || res["max_bytes"] != uint64(2000) || res["max_files"] != uint64(100) || res["physical_max_bytes"] != uint64(4000) {
		t.Fatalf("quota_set: %v %v", res, err)
	}

	var listed []invoke.Result
	err = s.InvokeStream(ctx, "quota_list", map[string]any{"min_bytes": "100"}, func(r invoke.Result) error {
		listed = append(listed, r)
		return nil
	})
	if err != nil || len(listed) != 1 || listed[0]["root"] != "/eos/user/b/bob" || listed[0]["mount_id"] != "eoshome-i01" {
		t.Fatalf("quota_list: %v %v", listed, err)
	}
}

func TestQuotaOfASingleSpaceMount(t *testing.T) {
	fs := &quotaFS{quotas: map[string]*storage.SpaceQuota{}}
	s := newQuotaService(fs, "/eos/project/c/cernbox", 2)
	if _, err := s.Invoke(context.Background(), "quota_get", map[string]any{"root": "/eos/project/c/cernbox", "space_type": "project"}); err != nil {
		t.Fatal(err)
	}
	if fs.refs[0].Root != "/" || fs.refs[0].Owner != nil {
		t.Fatalf("ref: %+v", fs.refs[0])
	}
	if _, err := s.Invoke(context.Background(), "quota_get", map[string]any{"root": "/eos/project/c/cernbox/sub", "space_type": "project"}); err == nil {
		t.Fatal("a folder in the space must be refused")
	}
}

func TestQuotaUnsupportedDriver(t *testing.T) {
	s := newQuotaService(struct{ storage.FS }{}, "/eos/user", 4)
	if _, err := s.Invoke(context.Background(), "quota_get", map[string]any{"root": "/eos/user/e/einstein", "space_type": "personal"}); err == nil {
		t.Fatal("a driver without quota administration must be reported")
	}
}

// The admin facade does not know the type of a space that is not in the
// catalogue, so the instance gives the type of its spaces.
func TestQuotaOfASpaceWithoutAType(t *testing.T) {
	fs := &quotaFS{quotas: map[string]*storage.SpaceQuota{}}
	s := newQuotaService(fs, "/eos/user", 4)
	s.conf.ProvidesSpaceType = "personal"
	res, err := s.Invoke(context.Background(), "quota_get", map[string]any{"root": "/eos/user/e/einstein"})
	if err != nil {
		t.Fatal(err)
	}
	if fs.refs[0].Type != "personal" || res["space_type"] != "personal" {
		t.Fatalf("ref: %+v, result: %v", fs.refs[0], res)
	}
}
