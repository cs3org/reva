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
	"os"

	preferencesv1beta1pb "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
)

var preferencesCommand = func() *command {
	cmd := newCommand("preferences")
	cmd.Description = func() string { return "set and get user preferences" }
	cmd.Usage = func() string { return "Usage: preferences set <key> <value> or preferences get <key>" }

	cmd.Action = func() error {

		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		subcommand := cmd.Args()[0]
		key := cmd.Args()[1]

		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		switch subcommand {
		case "set":
			if cmd.NArg() < 3 {
				fmt.Println(cmd.Usage())
				os.Exit(1)
			}
			value := cmd.Args()[2]
			req := &preferencesv1beta1pb.SetKeyRequest{
				Key: key,
				Val: value,
			}

			res, err := client.SetKey(ctx, req)
			if err != nil {
				return err
			}

			if res.Status.Code != rpcpb.Code_CODE_OK {
				return formatError(res.Status)
			}

		case "get":
			req := &preferencesv1beta1pb.GetKeyRequest{
				Key: key,
			}

			res, err := client.GetKey(ctx, req)
			if err != nil {
				return err
			}

			if res.Status.Code != rpcpb.Code_CODE_OK {
				return formatError(res.Status)
			}

			fmt.Println(res.Val)

		default:
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}
		return nil
	}
	return cmd
}
