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

	applicationsv1beta1 "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

func appTokensRemoveCommand() *command {
	cmd := newCommand("app-tokens-remove")
	cmd.Description = func() string { return "remove an application token" }
	cmd.Usage = func() string { return "Usage: token-remove <token>" }

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() != 1 {
			return errtypes.BadRequest("Invalid arguments: " + cmd.Usage())
		}

		token := cmd.Arg(0)
		ctx := getAuthContext()

		client, err := getClient()
		if err != nil {
			return err
		}

		response, err := client.InvalidateAppPassword(ctx, &applicationsv1beta1.InvalidateAppPasswordRequest{
			Password: token,
		})

		if err != nil {
			return err
		}

		if response.Status.Code != rpc.Code_CODE_OK {
			return formatError(response.Status)
		}

		fmt.Println("OK")
		return nil
	}

	return cmd
}
