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
	"strconv"

	"github.com/spf13/cobra"
)

func traceCommand() *cobra.Command {
	var user, since string
	var follow bool
	var tail int
	cmd := &cobra.Command{
		Use:   "trace [<traceid>]",
		Short: "Follow one request (by trace id) or one user across the whole fleet",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			// A trace id is grepped as a bare substring; a user by its structured
			// log field, which the auth layer stamps on every authenticated request.
			var grep string
			switch {
			case len(posArgs) > 0 && user != "":
				return errors.New("pass a trace id or --user, not both")
			case len(posArgs) > 0:
				grep = posArgs[0]
			case user != "":
				grep = fmt.Sprintf("%q:%q", "user", user)
			default:
				return errors.New("pass a trace id or --user")
			}
			args := map[string]string{"limit": strconv.Itoa(tail), "grep": grep}
			if since != "" {
				args["since"] = since
			}
			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}
			// Fan out to every instance in the fleet; each returns its own service's
			// matching lines, so the merged result is the request's full path.
			if follow {
				return followLogs(ctx, client, "*", args, rt.Output, true)
			}
			return snapshotLogs(ctx, client, "*", args, rt.Output, true)
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "trace by username instead of trace id")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new matching lines until interrupted")
	cmd.Flags().IntVarP(&tail, "lines", "n", 200, "number of recent lines per instance")
	cmd.Flags().StringVar(&since, "since", "", "only lines newer than a duration (e.g. 5m) or an RFC3339 time")
	return cmd
}
