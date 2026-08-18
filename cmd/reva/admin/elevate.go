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
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"google.golang.org/grpc/metadata"
)

func adminElevateCommand() *command {
	cmd := newCommand("elevate")
	cmd.Description = func() string { return "step up: exchange the login token for a short-TTL admin token" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	cmd.ResetFlags = func() { *adminHost = "" }
	cmd.Action = func(w ...io.Writer) error {
		host, err := resolveAdminHost(*adminHost)
		if err != nil {
			return err
		}
		client, err := adminClientAt(host)
		if err != nil {
			return err
		}
		userTok, err := cliOpts.LoginToken()
		if err != nil {
			return errors.New("no login token: run `login` first")
		}
		ctx := appctx.ContextSetToken(context.Background(), userTok)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, userTok)

		res, err := client.RequestAdmin(ctx, &adminpb.RequestAdminRequest{})
		if err != nil {
			return err
		}
		writeAdminToken(res.Token)
		fmt.Printf("elevated; admin token valid until %s\n", time.Unix(res.ExpiresAt, 0).Format(time.RFC3339))
		return nil
	}
	return cmd
}
