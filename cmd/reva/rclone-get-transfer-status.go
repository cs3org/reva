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

package main

// cli calling the rclone driver directly
import (
	"errors"
	"fmt"
	"io"
	"strconv"

	txdriver "github.com/cs3org/reva/pkg/datatx/driver"
	datatxConfig "github.com/cs3org/reva/pkg/datatx/driver/config"
	_ "github.com/cs3org/reva/pkg/datatx/driver/loader"
	registry "github.com/cs3org/reva/pkg/datatx/driver/registry"
)

func rcloneGetTransferStatus() *command {
	cmd := newCommand("rclone-get-transfer-status")
	cmd.Description = func() string { return "returns the rclone transfer status" }
	cmd.Usage = func() string { return "Usage: rclone-get-transfer-status [-flags]" }
	senderEndpoint := cmd.String("senderEndpoint", "", "rclone endpoint")
	transferID := cmd.String("transferID", "", "the job id")

	cmd.ResetFlags = func() {
		*senderEndpoint, *transferID = "", ""
	}

	cmd.Action = func(w ...io.Writer) error {

		// validate flags
		if *senderEndpoint == "" {
			return errors.New("sender endpoint must be specified: use -name flag\n" + cmd.Usage())
		}
		if *transferID == "" {
			return errors.New("transfer id must be specified: use -name flag\n" + cmd.Usage())
		}
		sndrEndpoint := fmt.Sprintf("\"senderEndpoint\":\"%v\"", *senderEndpoint)
		tID := fmt.Sprintf("\"job ID\":\"%v\"", *transferID)
		callParams := fmt.Sprintf("{%v}", tID)
		fmt.Printf("using: %v\n", sndrEndpoint)
		fmt.Printf("calling rclone.GetTransferStatus with params: %v\n", callParams)

		// rclone configuration
		c := &datatxConfig.Config{
			DataTxDriverType:     "rclone",
			DataTxSenderEndpoint: *senderEndpoint,
		}
		c.Init()

		rclone := registry.GetDriver(c.DataTxDriverType)

		err := rclone.Configure(c)
		if err != nil {
			return err
		}

		// prepare the input Job
		trID, err := strconv.ParseInt(*transferID, 10, 64)
		if err != nil {
			return err
		}
		job := &txdriver.Job{
			JobID: trID,
		}
		rcloneStatus, err := rclone.GetTransferStatus(job)

		fmt.Printf("received rclone job status: %v\n", rcloneStatus)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		return nil
	}
	return cmd
}
