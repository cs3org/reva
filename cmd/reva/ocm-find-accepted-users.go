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
	"encoding/gob"
	"io"
	"os"

	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/jedib0t/go-pretty/table"
)

func ocmFindAcceptedUsersCommand() *command {
	cmd := newCommand("ocm-find-accepted-users")
	cmd.Description = func() string { return "find remote users who have accepted invite tokens by their attributes" }
	cmd.Usage = func() string { return "Usage: ocm-find-accepted-users <filter>" }

	cmd.Action = func(w ...io.Writer) error {
		var filter string
		if cmd.NArg() == 1 {
			filter = cmd.Args()[0]
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		acceptedUsersRes, err := client.FindAcceptedUsers(ctx, &invitepb.FindAcceptedUsersRequest{
			Filter: filter,
		})
		if err != nil {
			return err
		}
		if acceptedUsersRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(acceptedUsersRes.Status)
		}

		if len(w) == 0 {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"OpaqueId", "Idp", "Mail", "DisplayName"})

			for _, u := range acceptedUsersRes.AcceptedUsers {
				t.AppendRows([]table.Row{
					{u.Id.OpaqueId, u.Id.Idp, u.Mail, u.DisplayName},
				})
			}
			t.Render()
		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(acceptedUsersRes.AcceptedUsers); err != nil {
				return err
			}
		}

		return nil
	}
	return cmd
}
