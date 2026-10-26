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
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func impersonateCommand() *cobra.Command {
	var (
		allowPrivileged bool
		ttl             time.Duration
	)
	cmd := &cobra.Command{
		Use:   "impersonate <user>",
		Short: "Print a user token for another user",
		Long: "Print a user token for another user, for a script or another program.\n" +
			"To run one reva command as another user, use --as instead.\n\n" +
			"--reason goes to the audit log.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if allowPrivileged {
				if err := rt.Confirm(fmt.Sprintf("impersonate %q even if it is an admin or a service account", args[0])); err != nil {
					return err
				}
			}
			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Impersonate(ctx, &adminpb.ImpersonateRequest{
				User: args[0], Reason: rt.Reason(), AllowPrivileged: allowPrivileged, TtlSeconds: int64(ttl.Seconds()),
			})
			if err != nil {
				return adminErr(err)
			}
			fmt.Println(res.Token)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allowPrivileged, "allow-privileged", false, "allow a target in admin_group or a service account (audited)")
	cmd.Flags().DurationVar(&ttl, "ttl", 0, "token lifetime, capped by the server at impersonation_ttl (default impersonation_ttl)")
	return cmd
}
