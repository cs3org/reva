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

package rclone

func init() {

}

// Rclone the rclone driver
type Rclone struct {
}

// DoTransfer initiates a transfer and returns the transfer job ID
func (driver *Rclone) DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (int64, error) {
	// example call from surfsara to cernbox
	//  - the users are to be defined with the remotes in the rclone config
	//  - basic http auth is used (-u user:pass)
	//
	// The example call:
	// curl
	// 	-u user:pass
	// 	-H "Content-Type: application/json"
	// 	-X POST
	// 	-d '{"srcFs":"surfsara:", "srcRemote":"/webdav/home/message-from-surfsara.txt", "dstFs":"cernbox:", "dstRemote":"/webdav/home/tmp/message-from-surfsara.txt", "_async":true}'
	// 	http://localhost:5572/operations/copyfile
	//
	//
	// 1. prepare config: add src/dest remotes
	// 2. do async call
	return -1, nil
}

// GetTransferStatus returns the status of the transfer with the specified job ID
func GetTransferStatus(jobID int64) (string, error) {
	//
	return "OK", nil
}

// CancelTransfer cancels the transfer with the specified job ID
func CancelTransfer(jobID int64) (bool, error) {
	return true, nil
}
