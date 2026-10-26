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

package eos

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
)

// quotaClient keeps quotas like an EOS instance with two replicas: it is set
// in physical bytes and reports logical bytes.
type quotaClient struct {
	eosclient.EOSClient
	quotas map[string]*eosclient.QuotaInfo
	sets   []string
}

func quotaKey(a eosclient.Authorization, node string) string {
	if a.Role.GID == eosclient.ProjectQuotaGID {
		return "gid=99 " + node
	}
	return "uid=" + a.Role.UID + " " + node
}

func (c *quotaClient) GetQuota(_ context.Context, user, _ eosclient.Authorization, path string) (*eosclient.QuotaInfo, error) {
	if qi, ok := c.quotas[quotaKey(user, path)]; ok {
		return qi, nil
	}
	return &eosclient.QuotaInfo{}, nil
}

func (c *quotaClient) SetQuota(_ context.Context, user, _ eosclient.Authorization, info *eosclient.SetQuotaInfo) error {
	k := quotaKey(user, info.QuotaNode)
	c.sets = append(c.sets, k)
	c.quotas[k] = &eosclient.QuotaInfo{TotalBytes: info.MaxBytes / 2, TotalInodes: info.MaxFiles}
	return nil
}

func (c *quotaClient) ListAllQuota(context.Context, eosclient.Authorization) (map[string]*eosclient.QuotaInfo, error) {
	return map[string]*eosclient.QuotaInfo{
		"/eos/user/e/einstein":   {TotalBytes: 10, UsedBytes: 4},
		"/eos/project/c/cernbox": {TotalBytes: 30},
	}, nil
}

func TestSpaceQuota(t *testing.T) {
	ctx := context.Background()
	c := &quotaClient{quotas: map[string]*eosclient.QuotaInfo{
		"gid=99 /eos/user/e/einstein": {TotalBytes: 10, UsedBytes: 4, TotalInodes: 100, UsedInodes: 7},
		"uid=1234 /eos/user/":         {TotalBytes: 20, UsedBytes: 5},
	}}
	fs := &Eosfs{
		c:          c,
		conf:       &Config{Namespace: "/eos/user/", QuotaNode: "/eos/user/", QuotaReplicaFactor: 2},
		userCache:  ttlcache.NewCache(),
		quotaCache: newQuotaCache(time.Minute),
	}
	_ = fs.userCache.Set("bob", &userpb.User{UidNumber: 1234, GidNumber: 2766})
	bob := &userpb.UserId{OpaqueId: "bob"}

	q, err := fs.GetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/e/einstein", Type: spaces.SpaceTypeHome, Owner: &userpb.UserId{OpaqueId: "einstein"}})
	if err != nil || q.Form != quotaFormProject || q.MaxBytes != 10 || q.PhysicalMaxBytes != 20 || q.UsedFiles != 7 || q.Root != "/e/einstein" {
		t.Fatalf("project quota: %+v %v", q, err)
	}

	// a space with no project quota falls back to the user quota of its owner
	q, err = fs.GetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/b/bob", Type: spaces.SpaceTypeHome, Owner: bob})
	if err != nil || q.Form != quotaFormUser || q.MaxBytes != 20 {
		t.Fatalf("user quota: %+v %v", q, err)
	}
	q, err = fs.GetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/b/bob", Type: spaces.SpaceTypeProject})
	if err != nil || q.Form != quotaFormProject || q.MaxBytes != 0 {
		t.Fatalf("no owner: %+v %v", q, err)
	}

	q, err = fs.SetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/b/bob", Type: spaces.SpaceTypeHome, Owner: bob}, 1000, 50)
	if err != nil || q.Form != quotaFormProject || q.MaxBytes != 1000 || q.PhysicalMaxBytes != 2000 || q.MaxFiles != 50 {
		t.Fatalf("set: %+v %v", q, err)
	}
	if len(c.sets) != 1 || c.sets[0] != "gid=99 /eos/user/b/bob" {
		t.Fatalf("sets: %v", c.sets)
	}
	if e, ok := fs.quotaCache.get("/eos/user/b/bob"); !ok || e.info.TotalBytes != 1000 {
		t.Fatalf("the quota cache is not refreshed: %+v", e)
	}

	if _, err := fs.SetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/b/bob", Type: spaces.SpaceTypeHome}, math.MaxUint64/2+1, 1); err == nil {
		t.Error("an overflowing quota must be refused")
	}
	if _, err := fs.GetSpaceQuota(ctx, storage.SpaceQuotaRef{Root: "/b/bob", Type: spaces.SpaceTypePublic}); err == nil {
		t.Error("a space type without quota must be refused")
	}

	qs, err := fs.ListSpaceQuotas(ctx)
	if err != nil || len(qs) != 1 || qs[0].Root != "/e/einstein" || qs[0].PhysicalMaxBytes != 20 {
		t.Fatalf("list: %+v %v", qs, err)
	}
}
