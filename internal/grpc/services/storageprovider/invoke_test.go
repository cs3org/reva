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

	"errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/storage"
)

func lookup(_ context.Context, username string) (*userpb.User, error) {
	if username == "einstein" {
		return &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 1234, GidNumber: 2766}, nil
	}
	return nil, errors.New("not found")
}

// quotaFS records what the invocations ask of the storage.
type quotaFS struct {
	storage.FS
	quotas map[string]*storage.QuotaInfo // keyed by uid|gid|path
	sets   []string
}

func key(q storage.QuotaRef) string { return q.UID + "|" + q.GID + "|" + q.Path }

func (f *quotaFS) GetQuotaFor(_ context.Context, q storage.QuotaRef) (*storage.QuotaInfo, error) {
	if qi, ok := f.quotas[key(q)]; ok {
		return qi, nil
	}
	return &storage.QuotaInfo{}, nil
}

func (f *quotaFS) SetQuotaFor(_ context.Context, q storage.QuotaRef, maxBytes, maxInodes uint64) error {
	f.sets = append(f.sets, key(q))
	f.quotas[key(q)] = &storage.QuotaInfo{TotalBytes: maxBytes, TotalInodes: maxInodes, UsedBytes: 1}
	return nil
}

func newService(fs storage.FS) *service {
	s := &service{conf: &config{Driver: "eoswrapper", ProvidesSpaceType: "project"}, storage: fs, mountPath: "/eos/project", mountID: "eosproject-c", lookup: lookup}
	s.registerInvocations()
	return s
}

func adminContext() context.Context {
	return appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "operator"}, Username: "operator"})
}

func TestQuotaInvocations(t *testing.T) {
	fs := &quotaFS{quotas: map[string]*storage.QuotaInfo{"|99|/c/cernbox": {TotalBytes: 10, UsedBytes: 2, TotalInodes: 100, UsedInodes: 3}}}
	s := newService(fs)

	if m := s.RegistryMetadata(); m["mount_id"] != "eosproject-c" || m["mount_path"] != "/eos/project" {
		t.Fatalf("metadata: %v", m)
	}
	res, err := s.Invoke(adminContext(), "mount", nil)
	if err != nil || res["mount_path"] != "/eos/project" || res["quota_admin"] != true {
		t.Fatalf("mount: %v %v", res, err)
	}

	// the project quota of a space, relative to the mount
	res, err = s.Invoke(adminContext(), "quota_get", map[string]any{"path": "/eos/project/c/cernbox"})
	if err != nil || res["total_bytes"] != uint64(10) || res["used_inodes"] != uint64(3) || res["gid"] != "99" || res["uid"] != "" {
		t.Fatalf("quota_get: %v %v", res, err)
	}
	// a user quota needs a known user and maps to the user's ids
	if _, err := s.Invoke(adminContext(), "quota_get", map[string]any{"path": "/eos/project/c/cernbox", "style": "uid"}); err == nil {
		t.Fatal("style=uid without a user must be refused")
	}
	res, err = s.Invoke(adminContext(), "quota_get", map[string]any{"path": "/eos/project", "style": "uid", "user": "einstein"})
	if err != nil || res["uid"] != "1234" || res["gid"] != "2766" {
		t.Fatalf("quota_get uid: %v %v", res, err)
	}
	// a path served elsewhere fails loudly, so does a relative one
	for _, p := range []string{"/eos/user/e/einstein", "/eos/projects", "c/cernbox", ""} {
		if _, err := s.Invoke(adminContext(), "quota_get", map[string]any{"path": p}); err == nil {
			t.Errorf("path %q must be refused", p)
		}
	}

	res, err = s.Invoke(adminContext(), "quota_set", map[string]any{"path": "/eos/project/c/cernbox", "max_bytes": "2000", "max_inodes": "50"})
	if err != nil || res["set"] != true || res["total_bytes"] != uint64(2000) || res["total_inodes"] != uint64(50) {
		t.Fatalf("quota_set: %v %v", res, err)
	}
	if len(fs.sets) != 1 || fs.sets[0] != "|99|/c/cernbox" {
		t.Fatalf("sets: %v", fs.sets)
	}
	for _, args := range []map[string]any{
		{"path": "/eos/project/c/cernbox", "max_bytes": "lots", "max_inodes": "1"},
		{"path": "/eos/project/c/cernbox", "max_bytes": "1"},
		{"path": "/eos/project/c/cernbox", "max_bytes": "1", "max_inodes": "1", "style": "bogus"},
	} {
		if _, err := s.Invoke(adminContext(), "quota_set", args); err == nil {
			t.Errorf("quota_set %v must fail", args)
		}
	}
}

func TestQuotaUnsupportedStorage(t *testing.T) {
	s := newService(struct{ storage.FS }{})
	if res, _ := s.Invoke(adminContext(), "mount", nil); res["quota_admin"] != false {
		t.Fatalf("mount: %v", res)
	}
	if _, err := s.Invoke(adminContext(), "quota_get", map[string]any{"path": "/eos/project/c/cernbox"}); err == nil {
		t.Fatal("a storage without quota administration must be reported")
	}
}
