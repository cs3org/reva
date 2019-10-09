// Copyright 2018-2019 CERN
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

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
)

func shareUpdateReceivedCommand() *command {
	cmd := newCommand("share-update-received")
	cmd.Description = func() string { return "update a received share" }
	cmd.Usage = func() string { return "Usage: share-update-received [-flags] <share_id>" }
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

		shareState := getShareState(*state)

		shareRequest := &usershareproviderv0alphapb.UpdateReceivedShareRequest{
			Ref: &usershareproviderv0alphapb.ShareReference{
				Spec: &usershareproviderv0alphapb.ShareReference_Id{
					Id: &usershareproviderv0alphapb.ShareId{
						OpaqueId: id,
					},
				},
			},
			Field: &usershareproviderv0alphapb.UpdateReceivedShareRequest_UpdateField{
				Field: &usershareproviderv0alphapb.UpdateReceivedShareRequest_UpdateField_State{
					State: shareState,
				},
			},
		}

		shareRes, err := shareClient.UpdateReceivedShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if shareRes.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(shareRes.Status)
		}

		fmt.Println("OK")
		return nil
	}
	return cmd
}

func getShareState(state string) usershareproviderv0alphapb.ShareState {
	switch state {
	case "pending":
		return usershareproviderv0alphapb.ShareState_SHARE_STATE_PENDING
	case "accepted":
		return usershareproviderv0alphapb.ShareState_SHARE_STATE_ACCEPTED
	case "rejected":
		return usershareproviderv0alphapb.ShareState_SHARE_STATE_REJECTED
	default:
		return usershareproviderv0alphapb.ShareState_SHARE_STATE_INVALID
	}
}
