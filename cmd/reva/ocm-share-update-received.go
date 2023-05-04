// Copyright 2018-2023 CERN
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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func ocmShareUpdateReceivedCommand() *command {
	cmd := newCommand("ocm-share-update-received")
	cmd.Description = func() string { return "update a received OCM share" }
	cmd.Usage = func() string { return "Usage: ocm-share-update-received [-flags] <share_id>" }
	state := cmd.String("state", "pending", "the state of the share (pending, accepted or rejected)")
	path := cmd.String("path", "", "the destination path of the data transfer (ignored if this is not a transfer type share)")

	cmd.ResetFlags = func() {
		*state = "pending"
		*path = ""
	}

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		// validate flags
		if *state != "pending" && *state != "accepted" && *state != "rejected" {
			return errors.New("Invalid state: state must be pending, accepted or rejected: " + cmd.Usage())
		}

		id := cmd.Args()[0]

		ctx := getAuthContext()
		shareClient, err := getClient()
		if err != nil {
			return err
		}

		shareState := getOCMShareState(*state)

		shareRes, err := shareClient.GetReceivedOCMShare(ctx, &ocm.GetReceivedOCMShareRequest{
			Ref: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Id{
					Id: &ocm.ShareId{
						OpaqueId: id,
					},
				},
			},
		})
		if err != nil {
			return err
		}
		if shareRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(shareRes.Status)
		}
		shareRes.Share.State = shareState

		// check if we are dealing with a transfer in case the destination path needs to be set
		_, ok := getTransferProtocol(shareRes.Share)
		var opaque *typesv1beta1.Opaque
		if ok {
			// transfer_destination_path is not part of TransferProtocol and is specified as an opaque field
			opaque = &typesv1beta1.Opaque{
				Map: map[string]*typesv1beta1.OpaqueEntry{
					"transfer_destination_path": {
						Decoder: "plain",
						Value:   []byte(*path),
					},
				},
			}
		}

		shareRequest := &ocm.UpdateReceivedOCMShareRequest{
			Share:      shareRes.Share,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
			Opaque:     opaque,
		}

		updateRes, err := shareClient.UpdateReceivedOCMShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if updateRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(updateRes.Status)
		}

		fmt.Println("OK")
		return nil
	}
	return cmd
}

func getTransferProtocol(share *ocm.ReceivedShare) (*ocm.TransferProtocol, bool) {
	for _, p := range share.Protocols {
		if d, ok := p.Term.(*ocm.Protocol_TransferOptions); ok {
			return d.TransferOptions, true
		}
	}
	return nil, false
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
