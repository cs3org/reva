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
	"errors"
	"fmt"
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func ocmShareGetReceivedCommand() *command {
	cmd := newCommand("ocm-share-get-received")
	cmd.Description = func() string { return "get the info of the OCM share you have received" }
	cmd.Usage = func() string { return "Usage: ocm-share-get-received" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		id := cmd.Arg(0)

		shareRes, err := client.GetReceivedOCMShare(ctx, &ocm.GetReceivedOCMShareRequest{
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

		d, err := utils.MarshalProtoV1ToJSON(shareRes.Share)
		if err != nil {
			return err
		}

		fmt.Println(string(d))

		return nil
	}
	return cmd
}
