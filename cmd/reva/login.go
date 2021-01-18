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
	"bufio"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"os"

	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/pkg/errors"
)

var loginCommand = func() *command {
	cmd := newCommand("login")
	cmd.Description = func() string { return "login into the reva server" }
	cmd.Usage = func() string { return "Usage: login <type>" }
	listFlag := cmd.Bool("list", false, "list available login methods")

	cmd.ResetFlags = func() {
		*listFlag = false
	}

	cmd.Action = func(w ...io.Writer) error {
		if *listFlag {
			// list available login methods
			client, err := getClient()
			if err != nil {
				return err
			}

			req := &registry.ListAuthProvidersRequest{}

			ctx := context.Background()
			res, err := client.ListAuthProviders(ctx, req)
			if err != nil {
				return err
			}

			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}

			if len(w) == 0 {
				fmt.Println("Available login methods:")
				for _, v := range res.Types {
					fmt.Printf("- %s\n", v)
				}
			} else {
				enc := gob.NewEncoder(w[0])
				if err := enc.Encode(res.Types); err != nil {
					return err
				}
			}
			return nil
		}

		if cmd.NArg() != 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		authType := cmd.Args()[0]
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("username: ")
		username, err := read(reader)
		if err != nil {
			return err
		}

		fmt.Print("password: ")
		password, err := readPassword(0)
		if err != nil {
			return err
		}

		client, err := getClient()
		if err != nil {
			return err
		}

		req := &gateway.AuthenticateRequest{
			Type:         authType,
			ClientId:     username,
			ClientSecret: password,
		}

		ctx := context.Background()
		res, err := client.Authenticate(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		writeToken(res.Token)
		fmt.Println("OK")
		return nil
	}
	return cmd
}
