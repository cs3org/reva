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
	"bufio"
	"context"
	"fmt"
	"os"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
)

var loginCommand = func() *command {
	cmd := newCommand("login")
	cmd.Description = func() string { return "login into the reva server" }
	cmd.Action = func() error {
		var username, password string
		if cmd.NArg() >= 2 {
			username = cmd.Args()[0]
			password = cmd.Args()[1]
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("username: ")
			usernameInput, err := read(reader)
			if err != nil {
				return err
			}

			fmt.Print("password: ")
			passwordInput, err := readPassword(0)
			if err != nil {
				return err
			}

			username = usernameInput
			password = passwordInput
		}

		client, err := getAuthClient()
		if err != nil {
			return err
		}

		req := &authv0alphapb.GenerateAccessTokenRequest{
			ClientId:     username,
			ClientSecret: password,
		}

		ctx := context.Background()
		res, err := client.GenerateAccessToken(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		writeToken(res.AccessToken)
		fmt.Println("OK")
		return nil
	}
	return cmd
}
