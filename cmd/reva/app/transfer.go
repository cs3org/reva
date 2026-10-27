// Copyright 2018-2026 CERN
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

package revacmd

import (
	"context"
	"io"
	"time"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func transferCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "transfer", Short: "Manage data transfers of OCM shares"}
	cmd.AddCommand(
		transferListCommand(rt),
		transferTxCommand(rt, "status", "Show the status of a transfer", backend.TransferManager.TransferStatus),
		transferTxCommand(rt, "cancel", "Cancel a running transfer", backend.TransferManager.CancelTransfer),
		transferTxCommand(rt, "retry", "Retry a transfer", backend.TransferManager.RetryTransfer),
	)
	return cmd
}

// transferTxCommand builds the commands that act on one transfer. They differ
// only in the call, and all print the transfer info that the call returns.
func transferTxCommand(rt *cli.Runtime, name, short string,
	call func(backend.TransferManager, context.Context, string) (*datatx.TxInfo, error)) *cobra.Command {
	return &cobra.Command{
		Use:   name + " TRANSFER_ID",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, err := cli.Ability[backend.TransferManager](cmd.Context(), rt, "OCM transfers")
			if err != nil {
				return err
			}
			info, err := call(tm, cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), info, func(w io.Writer) {
				t := cli.NewTable(w)
				t.AppendHeader(table.Row{"ShareId.OpaqueId", "Id.OpaqueId", "Status", "Ctime"})
				ctime := time.Unix(int64(info.Ctime.GetSeconds()), int64(info.Ctime.GetNanos()))
				t.AppendRow(table.Row{info.ShareId.GetOpaqueId(), info.Id.GetOpaqueId(), info.Status, ctime.Format(time.UnixDate)})
				t.Render()
			})
		},
	}
}

func transferListCommand(rt *cli.Runtime) *cobra.Command {
	var shareID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the transfers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, err := cli.Ability[backend.TransferManager](cmd.Context(), rt, "OCM transfers")
			if err != nil {
				return err
			}
			transfers, err := tm.ListTransfers(cmd.Context(), shareID)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), transfers, func(w io.Writer) {
				t := cli.NewTable(w)
				t.AppendHeader(table.Row{"ShareId.OpaqueId", "Id.OpaqueId"})
				for _, tx := range transfers {
					t.AppendRow(table.Row{tx.ShareId.GetOpaqueId(), tx.Id.GetOpaqueId()})
				}
				t.Render()
			})
		},
	}
	cmd.Flags().StringVar(&shareID, "share-id", "", "only the transfers of this OCM share")
	return cmd
}
