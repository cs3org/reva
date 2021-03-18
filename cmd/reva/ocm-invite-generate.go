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

	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

func ocmInviteGenerateCommand() *command {
	cmd := newCommand("ocm-invite-generate")
	cmd.Description = func() string { return "generate ocm invitation token" }
	cmd.Usage = func() string { return "Usage: ocm-invite-generate" }

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		inviteToken, err := client.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
		if err != nil {
			return err
		}
		if inviteToken.Status.Code != rpc.Code_CODE_OK {
			return formatError(inviteToken.Status)
		}
		fmt.Println(inviteToken)
		return nil
	}
	return cmd
}
