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

	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func elevateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "elevate",
		Short: "Exchange the login token for a short-TTL admin token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := resolveAdminHost()
			if err != nil {
				return err
			}
			client, err := adminClientAt(host)
			if err != nil {
				return err
			}
			userTok, err := rt.Token(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.RequestAdmin(cli.WithToken(cmd.Context(), userTok), &adminpb.RequestAdminRequest{})
			if err != nil {
				return err
			}
			if err := rt.SetAdminToken(res.Token); err != nil {
				return err
			}
			fmt.Printf("elevated; admin token valid until %s\n", time.Unix(res.ExpiresAt, 0).Format(time.RFC3339))
			return nil
		},
	}
}
