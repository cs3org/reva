// Copyright 2018-2019 CERN
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
	"log"
	"os"

	"github.com/cs3org/reva/pkg/storage/migrate"
)

func importCommand() *command {
	cmd := newCommand("import")
	cmd.Description = func() string { return "import metadata" }
	cmd.Usage = func() string { return "Usage: import [-flags] <user export folder>" }
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}
		exportPath := cmd.Args()[0]

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}
		if err := migrate.ImportShares(ctx, client, exportPath); err != nil {
			log.Fatal(err)
			return err
		}

		return nil
	}
	return cmd
}
