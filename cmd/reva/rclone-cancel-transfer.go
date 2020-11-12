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

	datatxConfig "github.com/cs3org/reva/pkg/datatx/driver/config"
	_ "github.com/cs3org/reva/pkg/datatx/driver/loader"
	registry "github.com/cs3org/reva/pkg/datatx/driver/registry"
)

func rcloneCancelTransfer() *command {
	cmd := newCommand("rclone-cancel-transfer")
	cmd.Description = func() string { return "cancels a transfer" }
	cmd.Usage = func() string { return "Usage: rclone-cancel-transfer [-flags]" }
	endpoint := cmd.String("endpoint", "", "rclone endpoint")
	transferID := cmd.String("transferID", "", "the job id")

	cmd.ResetFlags = func() {
		*endpoint, *transferID = "", ""
	}

	cmd.Action = func(w ...io.Writer) error {

		// validate flags
		if *endpoint == "" {
			return errors.New("sender endpoint must be specified: use -name flag\n" + cmd.Usage())
		}
		if *transferID == "" {
			return errors.New("transfer id must be specified: use -name flag\n" + cmd.Usage())
		}
		sndrEndpoint := fmt.Sprintf("\"endpoint\":\"%v\"", *endpoint)
		tID := fmt.Sprintf("\"job ID\":\"%v\"", *transferID)
		callParams := fmt.Sprintf("{%v}", tID)
		fmt.Printf("using: %v\n", sndrEndpoint)
		fmt.Printf("calling rclone.CancelTransfer with params: %v\n", callParams)

		// rclone configuration
		c := &datatxConfig.Config{
			Driver:   "rclone",
			Endpoint: *endpoint,
		}
		c.Init()

		rclone := registry.GetDriver(c.Driver)

		err := rclone.Configure(c)
		if err != nil {
			return err
		}

		// prepare the input Job
		trID, err := strconv.ParseInt(*transferID, 10, 64)
		if err != nil {
			return err
		}

		rcloneStatus, err := rclone.CancelTransfer(trID)

		fmt.Printf("received rclone job cancel status: %v\n", rcloneStatus)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		return nil
	}
	return cmd
}
