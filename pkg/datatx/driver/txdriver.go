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

package driver

// TxDriver the interface any transfer driver should implement
type TxDriver interface {
	// DoTransfer initiates a transfer and returns the transfer job ID
	DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (int64, error)
	// GetTransferStatus returns the status of the transfer with the specified job ID
	GetTransferStatus(jobID int64) (string, error)
	// CancelTransfer cancels the transfer with the specified job ID
	CancelTransfer(jobID int64) (bool, error)
}
