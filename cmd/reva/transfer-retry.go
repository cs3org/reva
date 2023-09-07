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
	"encoding/gob"
	"errors"
	"io"
	"os"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/jedib0t/go-pretty/table"
)

func transferRetryCommand() *command {
	cmd := newCommand("transfer-retry")
	cmd.Description = func() string { return "retry a transfer" }
	cmd.Usage = func() string { return "Usage: transfer-retry [-flags]" }
	txID := cmd.String("txId", "", "the transfer identifier")

	cmd.Action = func(w ...io.Writer) error {
		// validate flags
		if *txID == "" {
			return errors.New("txId must be specified: use -txId flag\n" + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		retryRequest := &datatx.RetryTransferRequest{
			TxId: &datatx.TxId{
				OpaqueId: *txID,
			},
		}

		retryResponse, err := client.RetryTransfer(ctx, retryRequest)
		if err != nil {
			return err
		}
		if retryResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(retryResponse.Status)
		}

		if len(w) == 0 {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"ShareId.OpaqueId", "Id.OpaqueId", "Status", "Ctime"})
			cTime := time.Unix(int64(retryResponse.TxInfo.Ctime.Seconds), int64(retryResponse.TxInfo.Ctime.Nanos))
			t.AppendRows([]table.Row{
				{retryResponse.TxInfo.ShareId.OpaqueId, retryResponse.TxInfo.Id.OpaqueId, retryResponse.TxInfo.Status, cTime},
			})
			t.Render()
		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(retryResponse.TxInfo); err != nil {
				return err
			}
		}

		return nil
	}
	return cmd
}
