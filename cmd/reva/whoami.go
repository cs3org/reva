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
	"context"
	"fmt"
	"os"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
)

func whoamiCommand() *command {
	cmd := newCommand("whoami")
	cmd.Description = func() string { return "tells who you are" }
	tokenFlag := cmd.String("token", "", "access token to use")

	cmd.Action = func() error {
		if cmd.NArg() != 0 {
			cmd.PrintDefaults()
			os.Exit(1)
		}
		var token string
		if *tokenFlag != "" {
			token = *tokenFlag
		} else {
			// read token from file
			t, err := readToken()
			if err != nil {
				fmt.Println("the token file cannot be readed from file ", getTokenFile())
				fmt.Println("make sure you have login before with \"reva login\"")
				return err
			}
			token = t
		}

		client, err := getAuthClient()
		if err != nil {
			return err
		}

		req := &authv0alphapb.WhoAmIRequest{AccessToken: token}

		ctx := context.Background()
		res, err := client.WhoAmI(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		me := res.User
		fmt.Printf("username: %s\ndisplay_name: %s\nmail: %s\ngroups: %v\n", me.Username, me.DisplayName, me.Mail, me.Groups)
		return nil
	}
	return cmd
}
