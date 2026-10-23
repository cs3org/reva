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
	"testing"

	"github.com/cs3org/reva/v3/pkg/projects"
)

func TestProjectsStats(t *testing.T) {
	catalogue, err, teardown := setupSuite(t)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown(t)
	mgr := catalogue.(*ProjectsManager)

	for _, p := range []*Project{
		{SpaceID: "a", Name: "a", Owner: "alice", Status: projects.ProjectStatusActive},
		{SpaceID: "b", Name: "b", Owner: "alice", Status: projects.ProjectStatusActive},
		{SpaceID: "c", Name: "c", Owner: "alice", Status: projects.ProjectStatusArchived},
		{SpaceID: "d", Name: "d", Owner: "bob", Status: projects.ProjectStatusActive},
	} {
		if err := mgr.db.Create(p).Error; err != nil {
			t.Fatal(err)
		}
	}

	p, err := mgr.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}

	got := map[[2]string]float64{}
	for _, m := range p.Metrics {
		if m.Name != "projects" {
			continue
		}
		for _, s := range m.OwnerSamples {
			got[[2]string{s.Owner, s.Labels["status"]}] = s.Value
		}
	}
	want := map[[2]string]float64{
		{"alice", "active"}:   2,
		{"alice", "archived"}: 1,
		{"bob", "active"}:     1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("projects of %s with status %s = %v, want %v", k[0], k[1], got[k], v)
		}
	}
}
