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

import (
	config "github.com/cs3org/reva/pkg/datatx/driver/config"
)

// StatusInvalid Invalid transfer status
const StatusInvalid string = "STATUS_INVALID"

// StatusDestinationNotFound The destination could not be found.
const StatusDestinationNotFound string = "STATUS_DESTINATION_NOT_FOUND"

// StatusTransferNew A new data transfer
const StatusTransferNew string = "STATUS_TRANSFER_NEW"

// // The data transfer is awaiting acceptance from the destination
// const STATUS_TRANSFER_AWAITING_ACCEPTANCE string = "STATUS_TRANSFER_AWAITING_ACCEPTANCE"

// // The data transfer is accepted by the destination.
// const STATUS_TRANSFER_ACCEPTED string = "STATUS_TRANSFER_ACCEPTED"

// StatusTransferInProgress The data transfer has started and not yet completed.
const StatusTransferInProgress string = "STATUS_TRANSFER_IN_PROGRESS"

// StatusTransferComplete The data transfer has completed.
const StatusTransferComplete string = "STATUS_TRANSFER_COMPLETE"

// StatusTransferFailed The data transfer has failed.
const StatusTransferFailed string = "STATUS_TRANSFER_FAILED"

// // The data transfer had been cancelled.
// const STATUS_TRANSFER_CANCELLED string = "STATUS_TRANSFER_CANCELLED"

// // The request for cancelling the data transfer has failed.
// const STATUS_TRANSFER_CANCEL_FAILED string = "STATUS_TRANSFER_CANCEL_FAILED"

// // The transfer has expired somewhere down the line.
// const STATUS_TRANSFER_EXPIRED string = "STATUS_TRANSFER_EXPIRED"

// Job the transfer job
type Job struct {
	JobID      int64
	SrcRemote  Remote
	DestRemote Remote
}

// Remote a remote in the transfer, either source or destination
type Remote struct {
	OpaqueID   int64
	OpaqueName string
}

// TxDriver the interface any transfer driver should implement
type TxDriver interface {
	// Configure configures the reader according to the specified configuration
	Configure(c *config.Config) error
	// DoTransfer initiates a transfer and returns the transfer job ID
	DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (*Job, error)
	// GetTransferStatus returns the status of the transfer defined by the specified job
	GetTransferStatus(job *Job) (string, error)
	// CancelTransfer cancels the transfer defined by the specified job
	CancelTransfer(job *Job) (bool, error)
}
