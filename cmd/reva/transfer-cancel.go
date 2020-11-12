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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

func transferCancelCommand() *command {
	cmd := newCommand("transfer-cancel")
	cmd.Description = func() string { return "cancel a running transfer" }
	cmd.Usage = func() string { return "Usage: transfer-cancel [-flags]" }
	txID := cmd.String("txID", "", "the transfer identifier")

	cmd.Action = func(w ...io.Writer) error {
		// validate flags
		if *txID == "" {
			return errors.New("txID must be specified: use -txID flag\n" + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		cancelRequest := &datatx.CancelTransferRequest{
			TxId: &datatx.TxId{
				OpaqueId: *txID,
			},
		}

		fmt.Println("transfer-cancel:")
		fmt.Println("------------------------------------------------------------------------")
		cancelResponse, err := client.CancelTransfer(ctx, cancelRequest)
		if err != nil {
			return err
		}
		if cancelResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(cancelResponse.Status)
		}

		fmt.Printf(" response status: %v\n", cancelResponse.Status)
		fmt.Printf(" transfer ID    : %v\n", cancelResponse.TxInfo.Id.OpaqueId)
		fmt.Printf(" transfer status: %v\n", cancelResponse.TxInfo.Status)
		fmt.Println("------------------------------------------------------------------------")

		return nil
	}
	return cmd
}
