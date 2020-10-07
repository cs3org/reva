// Copyright 2018-2020 CERN
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

	tx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/pkg/errors"
)

func (s *svc) CreateTransfer(ctx context.Context, req *tx.CreateTransferRequest) (*tx.CreateTransferResponse, error) {
	return &tx.CreateTransferResponse{
		Status: status.NewUnimplemented(ctx, errors.New("CreateTransfer not implemented"), "CreateTransfer not implemented"),
	}, nil
}

func (s *svc) GetTransferStatus(ctx context.Context, in *tx.GetTransferStatusRequest) (*tx.GetTransferStatusResponse, error) {
	return &tx.GetTransferStatusResponse{
		Status: status.NewUnimplemented(ctx, errors.New("GetTransfer not implemented"), "GetTransfer not implemented"),
	}, nil
}

func (s *svc) CancelTransfer(ctx context.Context, in *tx.CancelTransferRequest) (*tx.CancelTransferResponse, error) {
	return &tx.CancelTransferResponse{
		Status: status.NewUnimplemented(ctx, errors.New("CancelTransfer not implemented"), "CancelTransfer not implemented"),
	}, nil
}
