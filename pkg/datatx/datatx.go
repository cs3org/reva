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

package datatx

import (
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

// Manager the interface any transfer driver should implement
type Manager interface {
	// CreateTransfer initiates a transfer job and returns the new transfer status.
	// The already established transfer id is the reference to be used in other transfer interface methods.
	CreateTransfer(transferID string, srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (datatx.TxInfo_Status, error)
	// GetTransferStatus returns the current status of the transfer.
	GetTransferStatus(transferID string) (datatx.TxInfo_Status, error)
	// CancelTransfer cancels the transfer and returns its new status.
	CancelTransfer(transferID string) (datatx.TxInfo_Status, error)
}
