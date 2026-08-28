// Copyright 2018-2026 CERN
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

package admin

import (
	"errors"
	"fmt"
	"io"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminImpersonateCommand() *command {
	cmd := newCommand("impersonate")
	cmd.Description = func() string { return "mint a user-scoped token for a target user" }
	cmd.Usage = func() string { return "Usage: admin impersonate [-admin-host h] [-reason r] <user>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	reason := cmd.String("reason", "", "reason recorded in the audit log")
	cmd.ResetFlags = func() { *adminHost, *reason = "", "" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		res, err := client.Impersonate(ctx, &adminpb.ImpersonateRequest{User: cmd.Args()[0], Reason: *reason})
		if err != nil {
			return adminErr(err)
		}
		fmt.Println(res.Token)
		return nil
	}
	return cmd
}
