// Copyright 2018-2021 CERN
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

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cs3org/reva/cmd/reva/gen"
	"github.com/pkg/errors"
)

var genConfigSubCommand = func() *command {
	cmd := newCommand("config")
	cmd.Description = func() string { return "will create a revad.toml file" }
	cmd.Usage = func() string { return "Usage: gen config [-flags]" }

	forceFlag := cmd.Bool("f", false, "force")
	configFlag := cmd.String("c", "./revad.toml", "path to the config file")
	credentialsStrategyFlag := cmd.String("cs", "basic", "when initializing the config, choose 'basic' or 'oidc' credentials strategy")
	dataDriverFlag := cmd.String("dd", "local", "'local' or 'owncloud', ('s3' or 'eos' are supported when providing a custom config)")
	dataPathFlag := cmd.String("dp", "./data", "path to the data folder")

	cmd.ResetFlags = func() {
		*forceFlag, *configFlag, *credentialsStrategyFlag = false, "./revad.toml", "basic"
		*dataDriverFlag, *dataPathFlag = "local", "./data"
	}

	cmd.Action = func(w ...io.Writer) error {
		if !*forceFlag {
			if _, err := os.Stat(*configFlag); err == nil {
				// file exists, overwrite?
				fmt.Fprintf(os.Stdout, "%s exists, overwrite (y/N)? ", *configFlag)
				var r string
				_, err := fmt.Scanln(&r)
				if err != nil || "y" != strings.ToLower(r[:1]) {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if *credentialsStrategyFlag != "basic" && *credentialsStrategyFlag != "oidc" {
			return errors.New(fmt.Sprintf("unknown credentials strategy %s\n", *credentialsStrategyFlag))
		}
		if *dataDriverFlag == "local" || *dataDriverFlag == "owncloud" {
			gen.WriteConfig(*configFlag, *credentialsStrategyFlag, *dataDriverFlag, *dataPathFlag)
			if *credentialsStrategyFlag == "oidc" {
				fmt.Fprintf(os.Stdout, "make sure to serve phoenix on http://localhost:8300\n")
			}
			if *dataDriverFlag == "owncloud" {
				fmt.Fprintf(os.Stdout, "make sure to start a local redis server\n")
			}
			return nil
		} else if *dataDriverFlag == "eos" || *dataDriverFlag == "s3" {
			return errors.New(fmt.Sprintf("initializing %s configuration is not yet implemented\n", *dataDriverFlag))
		}
		return errors.New(fmt.Sprintf("unknown data driver %s\n", *dataDriverFlag))
	}
	return cmd
}
