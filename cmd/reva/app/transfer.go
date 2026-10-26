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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func transferCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "transfer", Short: "Manage data transfers of OCM shares"}
	cmd.AddCommand(
		transferListCommand(rt),
		transferTxCommand(rt, "status", "Show the status of a transfer",
			func(ctx context.Context, c gateway.GatewayAPIClient, id *datatx.TxId) (*rpc.Status, *datatx.TxInfo, error) {
				res, err := c.GetTransferStatus(ctx, &datatx.GetTransferStatusRequest{TxId: id})
				return res.GetStatus(), res.GetTxInfo(), err
			}),
		transferTxCommand(rt, "cancel", "Cancel a running transfer",
			func(ctx context.Context, c gateway.GatewayAPIClient, id *datatx.TxId) (*rpc.Status, *datatx.TxInfo, error) {
				res, err := c.CancelTransfer(ctx, &datatx.CancelTransferRequest{TxId: id})
				return res.GetStatus(), res.GetTxInfo(), err
			}),
		transferTxCommand(rt, "retry", "Retry a transfer",
			func(ctx context.Context, c gateway.GatewayAPIClient, id *datatx.TxId) (*rpc.Status, *datatx.TxInfo, error) {
				res, err := c.RetryTransfer(ctx, &datatx.RetryTransferRequest{TxId: id})
				return res.GetStatus(), res.GetTxInfo(), err
			}),
	)
	return cmd
}

// transferTxCommand builds the commands that act on one transfer. They differ
// only in the call, and all print the transfer info that the call returns.
func transferTxCommand(rt *cli.Runtime, name, short string,
	call func(context.Context, gateway.GatewayAPIClient, *datatx.TxId) (*rpc.Status, *datatx.TxInfo, error)) *cobra.Command {
	return &cobra.Command{
		Use:   name + " TRANSFER_ID",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			status, info, err := call(ctx, client, &datatx.TxId{OpaqueId: args[0]})
			if err != nil {
				return err
			}
			if status.Code != rpc.Code_CODE_OK {
				return formatError(status)
			}
			return rt.Print(cmd.OutOrStdout(), info, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
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
			req := &datatx.ListTransfersRequest{}
			if shareID != "" {
				req.Filters = []*datatx.ListTransfersRequest_Filter{{
					Type: datatx.ListTransfersRequest_Filter_TYPE_SHARE_ID,
					Term: &datatx.ListTransfersRequest_Filter_ShareId{ShareId: &ocm.ShareId{OpaqueId: shareID}},
				}}
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListTransfers(ctx, req)
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Transfers, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"ShareId.OpaqueId", "Id.OpaqueId"})
				for _, tx := range res.Transfers {
					t.AppendRow(table.Row{tx.ShareId.GetOpaqueId(), tx.Id.GetOpaqueId()})
				}
				t.Render()
			})
		},
	}
	cmd.Flags().StringVar(&shareID, "share-id", "", "only the transfers of this OCM share")
	return cmd
}
