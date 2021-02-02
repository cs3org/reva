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

// TxDriver the interface any transfer driver should implement
type TxDriver interface {
	// DoTransfer initiates a transfer and returns the transfer job id
	DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (int64, error)
	// GetTransferStatus returns the status of the transfer with the specified job id
	GetTransferStatus(jobID int64) (datatx.TxInfo_Status, error)
	// CancelTransfer cancels the transfer with the specified job id and returns the status
	CancelTransfer(jobID int64) (datatx.TxInfo_Status, error)
}
