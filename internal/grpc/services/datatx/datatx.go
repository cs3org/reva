// Copyright 2018-2021 CERN
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

package datatx

import (
	"context"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rgrpc"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("datatx", New)
}

type config struct {
}

type service struct {
	conf *config
}

func (c *config) init() {
}

func (s *service) Register(ss *grpc.Server) {
	datatx.RegisterTxAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new datatx svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	service := &service{
		conf: c,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) PullTransfer(ctx context.Context, req *datatx.PullTransferRequest) (*datatx.PullTransferResponse, error) {
	return &datatx.PullTransferResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("PullTransfer not implemented"), "PullTransfer not implemented"),
	}, nil
}

func (s *service) GetTransferStatus(ctx context.Context, in *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	return &datatx.GetTransferStatusResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("GetTransferStatus not implemented"), "GetTransferStatus not implemented"),
	}, nil
}

func (s *service) ListTransfers(ctx context.Context, in *datatx.ListTransfersRequest) (*datatx.ListTransfersResponse, error) {
	return &datatx.ListTransfersResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("ListTransfers not implemented"), "ListTransfers not implemented"),
	}, nil
}

func (s *service) RetryTransfer(ctx context.Context, in *datatx.RetryTransferRequest) (*datatx.RetryTransferResponse, error) {
	return &datatx.RetryTransferResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("RetryTransfer not implemented"), "RetryTransfer not implemented"),
	}, nil
}

func (s *service) CancelTransfer(ctx context.Context, in *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	return &datatx.CancelTransferResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CancelTransfer not implemented"), "CancelTransfer not implemented"),
	}, nil
}
