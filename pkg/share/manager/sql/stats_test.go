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
	"testing"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/stats"
)

// findSample returns the value of the sample matching the given labels in
// the named metric, and whether it was found.
func findSample(p *stats.Payload, name string, labels map[string]string) (float64, bool) {
	for _, m := range p.Metrics {
		if m.Name != name {
			continue
		}
	samples:
		for _, s := range m.Samples {
			for k, v := range labels {
				if s.Labels[k] != v {
					continue samples
				}
			}
			return s.Value, true
		}
	}
	return 0, false
}

func TestShareManagerStats(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)

	if _, err := mgr.Share(userctx, file, getUserShareGrant("1000", "file")); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Share(userctx, file, getUserShareGrant("1001", "file")); err != nil {
		t.Fatal(err)
	}

	reporter, ok := mgr.(stats.Reporter)
	if !ok {
		t.Fatal("sql share manager does not implement stats.Reporter")
	}
	p, err := reporter.Stats(userctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.Scope != stats.ScopeShared {
		t.Errorf("scope = %q, want shared", p.Scope)
	}

	if v, ok := findSample(p, "shares", map[string]string{
		"kind": "share", "status": "active", "item_type": "file",
	}); !ok || v != 2 {
		t.Errorf("active file shares = %v (found=%v), want 2", v, ok)
	}
	if v, ok := findSample(p, "shares_created", map[string]string{"kind": "share"}); !ok || v != 2 {
		t.Errorf("shares_created = %v (found=%v), want 2", v, ok)
	}
	if v, ok := findSample(p, "share_creators", map[string]string{"kind": "share"}); !ok || v != 1 {
		t.Errorf("share_creators = %v (found=%v), want 1", v, ok)
	}
	if v, ok := findSample(p, "share_recipients", map[string]string{"recipient_type": "user"}); !ok || v != 2 {
		t.Errorf("user recipients = %v (found=%v), want 2", v, ok)
	}
	if v, ok := findSample(p, "shares_per_recipient_max", nil); !ok || v != 1 {
		t.Errorf("per recipient max = %v (found=%v), want 1", v, ok)
	}
}

func TestPublicShareManagerStats(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)

	if _, err := mgr.CreatePublicShare(userctx, nil, file, getTestPublicLinkGrant(""), "no password", false, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreatePublicShare(userctx, nil, file, getTestPublicLinkGrant("secret"), "with password", false, false, ""); err != nil {
		t.Fatal(err)
	}

	reporter, ok := mgr.(stats.Reporter)
	if !ok {
		t.Fatal("sql public share manager does not implement stats.Reporter")
	}
	p, err := reporter.Stats(userctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}

	if v, ok := findSample(p, "shares", map[string]string{"kind": "link", "status": "active"}); !ok || v < 2 {
		t.Errorf("active links = %v (found=%v), want >= 2", v, ok)
	}
	if v, ok := findSample(p, "links", map[string]string{"password_protected": "true"}); !ok || v != 1 {
		t.Errorf("protected links = %v (found=%v), want 1", v, ok)
	}
	if v, ok := findSample(p, "links", map[string]string{"password_protected": "false"}); !ok || v != 1 {
		t.Errorf("open links = %v (found=%v), want 1", v, ok)
	}
	if v, ok := findSample(p, "shares_created", map[string]string{"kind": "link"}); !ok || v != 2 {
		t.Errorf("links created = %v (found=%v), want 2", v, ok)
	}
}
