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
	"io"
	"os"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/jedib0t/go-pretty/table"
)

func transferListCommand() *command {
	cmd := newCommand("transfer-list")
	cmd.Description = func() string { return "get a list of transfers" }
	cmd.Usage = func() string { return "Usage: transfer-list [-flags]" }
	filterShareID := cmd.String("shareId", "", "share ID filter (optional)")

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		// validate flags
		var filters []*datatx.ListTransfersRequest_Filter
		if *filterShareID != "" {
			filters = append(filters, &datatx.ListTransfersRequest_Filter{
				Type: datatx.ListTransfersRequest_Filter_TYPE_SHARE_ID,
				Term: &datatx.ListTransfersRequest_Filter_ShareId{
					ShareId: &ocm.ShareId{
						OpaqueId: *filterShareID,
					},
				},
			})
		}

		transferslistRequest := &datatx.ListTransfersRequest{
			Filters: filters,
		}

		listTransfersResponse, err := client.ListTransfers(ctx, transferslistRequest)
		if err != nil {
			return err
		}
		if listTransfersResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(listTransfersResponse.Status)
		}

		if len(w) == 0 {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"ShareId.OpaqueId", "Id.OpaqueId"})

			for _, s := range listTransfersResponse.Transfers {
				t.AppendRows([]table.Row{
					{s.ShareId.OpaqueId, s.Id.OpaqueId},
				})
			}
			t.Render()
		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(listTransfersResponse.Transfers); err != nil {
				return err
			}
		}

		return nil
	}
	return cmd
}
