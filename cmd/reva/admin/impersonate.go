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
	"fmt"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func impersonateCommand() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "impersonate <user>",
		Short: "Mint a user-scoped token for a target user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := adminDial(cmd)
			if err != nil {
				return err
			}
			res, err := client.Impersonate(ctx, &adminpb.ImpersonateRequest{User: args[0], Reason: reason})
			if err != nil {
				return adminErr(err)
			}
			fmt.Println(res.Token)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded in the audit log")
	return cmd
}
