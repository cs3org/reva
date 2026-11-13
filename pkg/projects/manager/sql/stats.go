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

	created, err := m.createdByStorage(ctx)
	if err != nil {
		return nil, err
	}
	return &stats.Payload{Metrics: []stats.Metric{projs, created}}, nil
}

// createdByStorage counts the projects ever created on each storage. It
// includes soft-deleted rows so the counter stays monotonic.
func (m *ProjectsManager) createdByStorage(ctx context.Context) (stats.Metric, error) {
	type row struct {
		StorageID string
		Count     int64
	}
	var rows []row
	err := m.db.WithContext(ctx).Model(&Project{}).Unscoped().
		Select("storage_id, COUNT(*) AS count").
		Group("storage_id").
		Find(&rows).Error
	if err != nil {
		return stats.Metric{}, err
	}
	created := stats.Metric{
		Name: "projects_created",
		Help: "Projects created, by storage.",
		Kind: stats.KindCounter,
	}
	for _, r := range rows {
		created.Samples = append(created.Samples, stats.Sample{
			Labels: map[string]string{"storage": r.StorageID},
			Value:  float64(r.Count),
		})
	}
	return created, nil
}
