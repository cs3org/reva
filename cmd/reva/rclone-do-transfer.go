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

	datatxConfig "github.com/cs3org/reva/pkg/datatx/driver/config"
	_ "github.com/cs3org/reva/pkg/datatx/driver/loader"
	registry "github.com/cs3org/reva/pkg/datatx/driver/registry"
)

func rcloneDoTransfer() *command {
	cmd := newCommand("rclone-do-transfer")
	cmd.Description = func() string { return "initiates an rclone transfer" }
	cmd.Usage = func() string { return "Usage: rclone-do-transfer [-flags]" }
	endpoint := cmd.String("endpoint", "", "rclone endpoint")
	srcEndpoint := cmd.String("srcEndpoint", "", "the source endpoint")
	srcToken := cmd.String("srcToken", "", "the token of the source user")
	srcPath := cmd.String("srcPath", "", "source path of the resource")
	destEndpoint := cmd.String("destEndpoint", "", "the destination endpoint")
	destPath := cmd.String("destPath", "", "destination path of the resource")
	destToken := cmd.String("destToken", "", "the token of the destination user")

	cmd.ResetFlags = func() {
		*endpoint, *srcEndpoint, *srcPath, *srcToken, *destEndpoint, *destPath, *destToken = "", "", "", "", "", "", ""
	}

	cmd.Action = func(w ...io.Writer) error {

		// validate flags
		if *endpoint == "" {
			return errors.New("sender endpoint must be specified: use -name flag\n" + cmd.Usage())
		}
		if *srcEndpoint == "" {
			return errors.New("source endpoint must be specified: use -name flag\n" + cmd.Usage())
		}
		if *srcPath == "" {
			return errors.New("source path must be specified: use -name flag\n" + cmd.Usage())
		}
		if *srcToken == "" {
			return errors.New("source token must be specified: use -name flag\n" + cmd.Usage())
		}
		if *destEndpoint == "" {
			return errors.New("destination endpoint must be specified: use -name flag\n" + cmd.Usage())
		}
		if *destPath == "" {
			return errors.New("destination path must be specified: use -name flag\n" + cmd.Usage())
		}
		if *destToken == "" {
			return errors.New("destination token must be specified: use -name flag\n" + cmd.Usage())
		}
		sndrEndpoint := fmt.Sprintf("\"endpoint\":\"%v\"", *endpoint)
		sourceEndpoint := fmt.Sprintf("\"srcEndpoint\":\"%v\"", *srcEndpoint)
		sourcePath := fmt.Sprintf("\"srcPath\":\"%v\"", *srcPath)
		sourceToken := fmt.Sprintf("\"srcToken\":\"%v\"", *srcToken)
		destinationEndpoint := fmt.Sprintf("\"destEndpoint\":\"%v\"", *destEndpoint)
		destinationPath := fmt.Sprintf("\"destPath\":\"%v\"", *destPath)
		destinationToken := fmt.Sprintf("\"destToken\":\"%v\"", *destToken)
		callParams := fmt.Sprintf("{%v, %v, %v, %v, %v, %v}", sourceEndpoint, sourcePath, sourceToken, destinationEndpoint, destinationPath, destinationToken)
		fmt.Printf("using: %v\n", sndrEndpoint)
		fmt.Printf("calling rclone.DoTransfer with params: %v\n", callParams)

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

		jobID, err := rclone.DoTransfer(*srcEndpoint, *srcPath, *srcToken, *destEndpoint, *destPath, *destToken)
		if err != nil {
			return err
		}

		fmt.Printf("received rclone job id: %v\n", jobID)

		return nil
	}
	return cmd
}
