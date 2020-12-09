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
	"errors"
	"fmt"
	"io"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	tx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

func transferCreateCommand() *command {
	cmd := newCommand("transfer-create")
	cmd.Description = func() string { return "create transfer between 2 sites" }
	cmd.Usage = func() string { return "Usage: transfer-create [-flags] <path>" }
	grantee := cmd.String("grantee", "", "the grantee, receiver of the transfer")
	granteeType := cmd.String("granteeType", "user", "the grantee type, one of: user, group")
	idp := cmd.String("idp", "", "the idp of the grantee, default to same idp as the user triggering the action")

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		if *grantee == "" {
			return errors.New("Grantee cannot be empty: use -grantee flag\n" + cmd.Usage())
		}
		if *idp == "" {
			return errors.New("Idp cannot be empty: use -idp flag\n" + cmd.Usage())
		}

		// the resource to transfer; the path
		fn := cmd.Args()[0]

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		gt := provider.GranteeType_GRANTEE_TYPE_USER
		if strings.ToLower(*granteeType) == "group" {
			gt = provider.GranteeType_GRANTEE_TYPE_GROUP
		}

		transferRequest := &tx.CreateTransferRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: fn,
				},
			},
			Grantee: &provider.Grantee{
				Type: gt,
				Id: &userpb.UserId{
					Idp:      *idp,
					OpaqueId: *grantee,
				},
			},
		}

		fmt.Println("transfer-create:")
		fmt.Println("------------------------------------------------------------------------")
		transferResponse, err := client.CreateTransfer(ctx, transferRequest)
		if err != nil {
			return err
		}
		if transferResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(transferResponse.Status)
		}

		fmt.Printf(" response status: %v\n", transferResponse.Status)
		fmt.Printf(" transfer ID    : %v\n", transferResponse.TxInfo.Id.OpaqueId)
		fmt.Printf(" transfer status: %v\n", transferResponse.TxInfo.Status)
		fmt.Println("------------------------------------------------------------------------")

		return nil
	}
	return cmd
}
