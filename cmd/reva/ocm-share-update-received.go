// Copyright 2018-2020 CERN
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
	"os"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
)

func ocmShareUpdateReceivedCommand() *command {
	cmd := newCommand("ocm-share-update-received")
	cmd.Description = func() string { return "update a received OCM share" }
	cmd.Usage = func() string { return "Usage: ocm-share-update-received [-flags] <share_id>" }
	state := cmd.String("state", "pending", "the state of the share (pending, accepted or rejected)")
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		// validate flags
		if *state != "pending" && *state != "accepted" && *state != "rejected" {
			fmt.Println("invalid state: state must be pending, accepted or rejected")
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		id := cmd.Args()[0]

		ctx := getAuthContext()
		shareClient, err := getClient()
		if err != nil {
			return err
		}

		shareState := getOCMShareState(*state)

		shareRequest := &ocm.UpdateReceivedOCMShareRequest{
			Ref: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Id{
					Id: &ocm.ShareId{
						OpaqueId: id,
					},
				},
			},
			Field: &ocm.UpdateReceivedOCMShareRequest_UpdateField{
				Field: &ocm.UpdateReceivedOCMShareRequest_UpdateField_State{
					State: shareState,
				},
			},
		}

		shareRes, err := shareClient.UpdateReceivedOCMShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if shareRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(shareRes.Status)
		}

		fmt.Println("OK")
		return nil
	}
	return cmd
}

func getOCMShareState(state string) ocm.ShareState {
	switch state {
	case "pending":
		return ocm.ShareState_SHARE_STATE_PENDING
	case "accepted":
		return ocm.ShareState_SHARE_STATE_ACCEPTED
	case "rejected":
		return ocm.ShareState_SHARE_STATE_REJECTED
	default:
		return ocm.ShareState_SHARE_STATE_INVALID
	}
}
