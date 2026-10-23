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

	"github.com/cs3org/reva/v3/pkg/stats"
)

// Stats implements stats.Reporter: the projects of each owner, by status.
func (m *ProjectsManager) Stats(ctx context.Context) (*stats.Payload, error) {
	type row struct {
		Owner  string
		Status string
		Count  int64
	}
	var rows []row
	err := m.db.WithContext(ctx).Model(&Project{}).
		Select("owner, status, COUNT(*) AS count").
		Group("owner, status").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	projs := stats.Metric{
		Name: "projects",
		Help: "Projects, by status and owner attributes.",
		Kind: stats.KindGauge,
	}
	for _, r := range rows {
		labels := map[string]string{"status": r.Status}
		if r.Owner == "" {
			projs.Samples = append(projs.Samples, stats.Sample{Labels: labels, Value: float64(r.Count)})
			continue
		}
		projs.OwnerSamples = append(projs.OwnerSamples, stats.OwnerSample{Owner: r.Owner, Labels: labels, Value: float64(r.Count)})
	}
	return &stats.Payload{Scope: stats.ScopeShared, Metrics: []stats.Metric{projs}}, nil
}
