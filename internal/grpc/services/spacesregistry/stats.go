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

package spacesregistry

import (
	"context"
	"sync"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/projects"
	projectssql "github.com/cs3org/reva/v3/pkg/projects/manager/sql"
	"github.com/cs3org/reva/v3/pkg/stats"
	"golang.org/x/sync/errgroup"
)

// quotaConcurrency bounds the parallel quota calls, so a refresh does not
// flood the storage with one request per project at the same time.
const quotaConcurrency = 16

// allProjectsLister is a catalogue that lists every project, not only the
// projects of the user in the context.
type allProjectsLister interface {
	ListAllProjects(ctx context.Context, status projects.ProjectStatus, owner string) ([]*projectssql.Project, error)
}

// Stats adds the quota and used space of the active projects, per owner,
// to the statistics of the catalogue.
func (s *service) Stats(ctx context.Context) (*stats.Payload, error) {
	p, err := s.projects.(stats.Reporter).Stats(ctx)
	if err != nil {
		return nil, err
	}
	lister, ok := s.projects.(allProjectsLister)
	if !ok {
		return p, nil
	}
	projs, err := lister.ListAllProjects(ctx, projects.ProjectStatusActive, "")
	if err != nil {
		return nil, err
	}

	quota := stats.Metric{
		Name: "project_quota_bytes",
		Help: "Quota of the active projects, by owner attributes.",
		Kind: stats.KindGauge,
	}
	used := stats.Metric{
		Name: "project_used_bytes",
		Help: "Used space of the active projects, by owner attributes.",
		Kind: stats.KindGauge,
	}
	var (
		mu     sync.Mutex
		failed int
		g      errgroup.Group
	)
	g.SetLimit(quotaConcurrency)
	for _, proj := range projs {
		g.Go(func() error {
			qctx, cancel := context.WithTimeout(ctx, s.timeoutSkipSpaces)
			defer cancel()
			q, err := s.projectQuota(qctx, proj.Owner, proj.Path)

			mu.Lock()
			defer mu.Unlock()
			if err != nil || q.GetStatus().GetCode() != rpcv1beta1.Code_CODE_OK {
				failed++
				return nil
			}
			if proj.Owner == "" {
				quota.Samples = append(quota.Samples, stats.Sample{Value: float64(q.TotalBytes)})
				used.Samples = append(used.Samples, stats.Sample{Value: float64(q.UsedBytes)})
				return nil
			}
			quota.OwnerSamples = append(quota.OwnerSamples, stats.OwnerSample{Owner: proj.Owner, Value: float64(q.TotalBytes)})
			used.OwnerSamples = append(used.OwnerSamples, stats.OwnerSample{Owner: proj.Owner, Value: float64(q.UsedBytes)})
			return nil
		})
	}
	_ = g.Wait()
	if failed > 0 {
		// One line per refresh: a line per project would flood the log.
		appctx.GetLogger(ctx).Warn().Int("failed", failed).Int("projects", len(projs)).Msg("spacesregistry: stats: no quota for some projects")
	}

	p.Metrics = append(p.Metrics, quota, used)
	return p, nil
}
