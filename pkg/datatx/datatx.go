// Copyright 2018-2023 CERN
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

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

// Manager the interface any transfer driver should implement.
type Manager interface {
	// CreateTransfer creates a transfer job and returns a TxInfo object that includes a unique transfer id.
	// Specified target URIs are of form scheme://userinfo@host:port?name={path}
	CreateTransfer(ctx context.Context, srcTargetURI string, dstTargetURI string) (*datatx.TxInfo, error)
	// GetTransferStatus returns a TxInfo object including the current status, and error if any.
	GetTransferStatus(ctx context.Context, transferID string) (*datatx.TxInfo, error)
	// CancelTransfer cancels the transfer and returns a TxInfo object and error if any.
	CancelTransfer(ctx context.Context, transferID string) (*datatx.TxInfo, error)
	// RetryTransfer retries the transfer and returns a TxInfo object and error if any.
	// Note that tokens must still be valid.
	RetryTransfer(ctx context.Context, transferID string) (*datatx.TxInfo, error)
}

// Transfer represents datatx transfer.
type Transfer struct {
	TxID          string
	SrcTargetURI  string
	DestTargetURI string
	ShareID       string
	UserID        *userv1beta1.UserId
}

// Repository the interface that any storage driver should implement.
type Repository interface {
	// StoreTransfer stores the transfer by its TxID
	StoreTransfer(transfer *Transfer) error
	// StoreTransfer deletes the transfer by its TxID
	DeleteTransfer(transfer *Transfer) error
	// GetTransfer returns the transfer with the specified transfer id
	GetTransfer(txID string) (*Transfer, error)
	// ListTransfers returns a filtered list of transfers
	ListTransfers(Filters []*datatx.ListTransfersRequest_Filter, UserID *userv1beta1.UserId) ([]*Transfer, error)
}
