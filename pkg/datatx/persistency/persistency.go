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

package persistency

// Transfer the transfer persistency struct.
type Transfer struct {
	TransferID     int64
	JobID          int64
	TransferStatus int32
	SrcRemote      string
	SrcPath        string
	DestRemote     string
	DestPath       string
}

// Driver the persistency driver interface
type Driver interface {
	// SaveTransfer saves a datatx transfer; for both new transfers and existing transfer updates.
	// If a transfer is new and does not have a transfer id yet it is the responsibility of the driver
	// to create a new transfer id and set it in the returned Transfer.
	SaveTransfer(transfer *Transfer) (*Transfer, error)
	// GetTransfer returns the transfer with the specified transfer id.
	GetTransfer(transferID int64) (*Transfer, error)
}
