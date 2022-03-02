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

package gateway

import (
	"context"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) PullTransfer(ctx context.Context, req *datatx.PullTransferRequest) (*datatx.PullTransferResponse, error) {
	c, err := pool.GetDataTxClient(s.c.DataTxEndpoint)
	if err != nil {
		return &datatx.PullTransferResponse{
			Status: status.NewInternal(ctx, "error getting data transfer client"),
		}, nil
	}

	res, err := c.PullTransfer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateTransfer")
	}

	return res, nil
}

func (s *svc) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	c, err := pool.GetDataTxClient(s.c.DataTxEndpoint)
	if err != nil {
		return &datatx.GetTransferStatusResponse{
			Status: status.NewInternal(ctx, "error getting data transfer client"),
		}, nil
	}

	res, err := c.GetTransferStatus(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetTransferStatus")
	}

	return res, nil
}

func (s *svc) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	c, err := pool.GetDataTxClient(s.c.DataTxEndpoint)
	if err != nil {
		return &datatx.CancelTransferResponse{
			Status: status.NewInternal(ctx, "error getting data transfer client"),
		}, nil
	}

	res, err := c.CancelTransfer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CancelTransfer")
	}

	return res, nil
}

func (s *svc) ListTransfers(ctx context.Context, req *datatx.ListTransfersRequest) (*datatx.ListTransfersResponse, error) {
	return &datatx.ListTransfersResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("ListTransfers not implemented"), "ListTransfers not implemented"),
	}, nil
}

func (s *svc) RetryTransfer(ctx context.Context, req *datatx.RetryTransferRequest) (*datatx.RetryTransferResponse, error) {
	return &datatx.RetryTransferResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("RetryTransfer not implemented"), "RetryTransfer not implemented"),
	}, nil
}
