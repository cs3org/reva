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

package openapi

import (
	"context"
	"encoding/json"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/service"
)

// fleet is the set of HTTP services registered in the deployment. This
// interface abstracts it for testability.
type fleet interface {
	// Documents returns the OpenAPI document of each service, by name.
	Documents(ctx context.Context) (map[string][]byte, error)
}

// adminFleet is the production fleet, reached through the Admin API as the
// service openapi.
type adminFleet struct{}

func (adminFleet) Documents(ctx context.Context) (map[string][]byte, error) {
	adm, err := service.Admin(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = admin.AsService(ctx, "openapi")
	if err != nil {
		return nil, err
	}
	res, err := adm.ListServices(ctx, &adminpb.ListServicesRequest{Invocation: invoke.APIInvocation})
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	out := make(map[string][]byte, len(res.Services))
	for _, sv := range res.Services {
		// The invocation is shared, so the Admin API asks one instance.
		inv, err := adm.Invoke(ctx, &adminpb.InvokeRequest{Service: sv.Name, Invocation: invoke.APIInvocation})
		if err != nil {
			log.Warn().Err(err).Str("service", sv.Name).Msg("openapi: asking a service for its document")
			continue
		}
		for _, r := range inv.Results {
			if r.Error != "" {
				log.Warn().Str("node", r.Node).Str("error", r.Error).Msg("openapi: a node could not describe its routes")
				continue
			}
			var result struct {
				Document json.RawMessage `json:"document"`
			}
			if err := json.Unmarshal([]byte(r.ResultJson), &result); err != nil || len(result.Document) == 0 {
				log.Warn().Err(err).Str("node", r.Node).Msg("openapi: a node returned no document")
				continue
			}
			out[sv.Name] = result.Document
			break
		}
	}
	return out, nil
}
