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

package stats

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/stats"
)

// fleet is the set of reva services registered in the deployment, as the
// Admin API sees it. This interface abstracts it for testability.
type fleet interface {
	// StatsServices returns the services advertising the stats invocation.
	StatsServices(ctx context.Context) ([]string, error)
	// InvokeStats runs the stats invocation on a service and returns the
	// payloads that its instances streamed.
	InvokeStats(ctx context.Context, service string) ([]*stats.Payload, error)
}

// adminFleet is the production fleet, reached through the Admin API as
// the service stats-collector.
type adminFleet struct{}

// StatsServices asks the Admin API for the services that advertise the
// stats invocation.
func (adminFleet) StatsServices(ctx context.Context) ([]string, error) {
	adm, err := service.Admin(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = admin.AsService(ctx, "stats-collector")
	if err != nil {
		return nil, err
	}
	res, err := adm.ListServices(ctx, &adminpb.ListServicesRequest{Invocation: stats.Invocation})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Services))
	for _, sv := range res.Services {
		names = append(names, sv.Name)
	}
	return names, nil
}

// invokeTimeout bounds the stats stream of one service. The Admin API sets
// no time limit on a stream, and a stats invocation can aggregate large
// tables or call storage once per project.
const invokeTimeout = 5 * time.Minute

// InvokeStats streams stats through the Admin API, which routes the call:
// one instance for a shared invocation, every instance otherwise.
func (adminFleet) InvokeStats(ctx context.Context, svc string) ([]*stats.Payload, error) {
	adm, err := service.Admin(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = admin.AsService(ctx, "stats-collector")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, invokeTimeout)
	defer cancel()
	stream, err := adm.InvokeStream(ctx, &adminpb.InvokeRequest{Service: svc, Invocation: stats.Invocation})
	if err != nil {
		return nil, err
	}
	var payloads []*stats.Payload
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return payloads, nil
		}
		if err != nil {
			return nil, err
		}
		if msg.Error != "" {
			return nil, fmt.Errorf("%s: %s", msg.Node, msg.Error)
		}
		p, err := stats.FromJSON([]byte(msg.ResultJson))
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, p)
	}
}
