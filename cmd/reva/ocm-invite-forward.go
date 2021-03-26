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
	"errors"
	"fmt"
	"io"

	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

func ocmInviteForwardCommand() *command {
	cmd := newCommand("ocm-invite-forward")
	cmd.Description = func() string { return "forward ocm invite token" }
	cmd.Usage = func() string { return "Usage: ocm-invite-forward [-flags] <path>" }
	token := cmd.String("token", "", "invite token")
	idp := cmd.String("idp", "", "the idp of the user who generated the token")

	cmd.ResetFlags = func() {
		*token, *idp = "", ""
	}

	cmd.Action = func(w ...io.Writer) error {
		// validate flags
		if *token == "" {
			return errors.New("token cannot be empty: use -token flag\n" + cmd.Usage())
		}
		if *idp == "" {
			return errors.New("Provider domain cannot be empty: use -provider flag\n" + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		inviteToken := &invitepb.InviteToken{
			Token: *token,
		}

		providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
			Domain: *idp,
		})
		if err != nil {
			return err
		}

		if providerInfo.Status.Code != rpc.Code_CODE_OK {
			return formatError(providerInfo.Status)
		}

		forwardToken, err := client.ForwardInvite(ctx, &invitepb.ForwardInviteRequest{
			InviteToken:          inviteToken,
			OriginSystemProvider: providerInfo.ProviderInfo,
		})
		if err != nil {
			return err
		}

		if forwardToken.Status.Code != rpc.Code_CODE_OK {
			return formatError(forwardToken.Status)
		}
		fmt.Println("OK")
		return nil
	}
	return cmd
}
