// Copyright 2018-2019 CERN
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

package gatewaysvc

import (
	"context"

	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/pkg/errors"
)

func (s *svc) SetKey(ctx context.Context, req *preferencesv0alphapb.SetKeyRequest) (*preferencesv0alphapb.SetKeyResponse, error) {
	c, err := pool.GetPreferencesClient(s.c.PreferencesEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetPreferencesClient")
		return &preferencesv0alphapb.SetKeyResponse{
			Status: status.NewInternal(ctx, err, "error getting preferences client"),
		}, nil
	}

	res, err := c.SetKey(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling SetKey")
	}

	return res, nil
}

func (s *svc) GetKey(ctx context.Context, req *preferencesv0alphapb.GetKeyRequest) (*preferencesv0alphapb.GetKeyResponse, error) {
	c, err := pool.GetPreferencesClient(s.c.PreferencesEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetPreferencesClient")
		return &preferencesv0alphapb.GetKeyResponse{
			Status: status.NewInternal(ctx, err, "error getting preferences client"),
		}, nil
	}

	res, err := c.GetKey(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetKey")
	}

	return res, nil
}
