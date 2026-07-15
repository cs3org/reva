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
	"strconv"
)

func adminTraceCommand() *command {
	cmd := newCommand("trace")
	cmd.Description = func() string { return "follow one request (by trace id) or one user across the whole fleet" }
	cmd.Usage = func() string {
		return "Usage: admin trace [-admin-host h] [-user u] [-f] [-n N] [-since D] [-o text|json] [<traceid>]"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	user := cmd.String("user", "", "trace by username instead of trace id")
	follow := cmd.Bool("f", false, "follow: stream new matching lines until interrupted")
	tail := cmd.Int("n", 200, "number of recent lines per instance")
	since := cmd.String("since", "", "only lines newer than a duration (e.g. 5m) or an RFC3339 time")
	output := cmd.String("o", "text", "output format: text | json")
	cmd.ResetFlags = func() { *adminHost, *user, *follow, *tail, *since, *output = "", "", false, 200, "", "text" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 && *user == "" {
			return errors.New(cmd.Usage())
		}
		if *output != "text" && *output != "json" {
			return fmt.Errorf("unknown -o %q (use text or json)", *output)
		}
		// A trace id is grepped as a bare substring; a user by its structured
		// log field, which the auth layer stamps on every authenticated request.
		grep := ""
		if cmd.NArg() > 0 {
			grep = cmd.Args()[0]
		}
		if *user != "" {
			if grep != "" {
				return errors.New("pass a trace id or -user, not both")
			}
			grep = fmt.Sprintf("%q:%q", "user", *user)
		}
		args := map[string]string{"limit": strconv.Itoa(*tail), "grep": grep}
		if *since != "" {
			args["since"] = *since
		}

		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		// Fan out to every instance in the fleet; each returns its own service's
		// matching lines, so the merged result is the request's full path.
		if *follow {
			return followLogs(ctx, client, "*", args, *output, true)
		}
		return snapshotLogs(ctx, client, "*", args, *output, true)
	}
	return cmd
}
