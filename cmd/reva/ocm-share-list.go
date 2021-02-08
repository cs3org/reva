// Copyright 2018-2021 CERN
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
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/jedib0t/go-pretty/table"
)

func ocmShareListCommand() *command {
	cmd := newCommand("ocm-share-list")
	cmd.Description = func() string { return "list OCM shares you manage" }
	cmd.Usage = func() string { return "Usage: ocm-share-list [-flags]" }
	resID := cmd.String("by-resource-id", "", "filter by resource id (storage_id:opaque_id)")

	cmd.ResetFlags = func() {
		*resID = ""
	}

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()
		shareClient, err := getClient()
		if err != nil {
			return err
		}

		shareRequest := &ocm.ListOCMSharesRequest{}
		if *resID != "" {
			// check split by colon (:)
			tokens := strings.Split(*resID, ":")
			if len(tokens) != 2 {
				return fmt.Errorf("resource id invalid")
			}
			id := &provider.ResourceId{
				StorageId: tokens[0],
				OpaqueId:  tokens[1],
			}
			shareRequest.Filters = []*ocm.ListOCMSharesRequest_Filter{
				&ocm.ListOCMSharesRequest_Filter{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
					Term: &ocm.ListOCMSharesRequest_Filter_ResourceId{
						ResourceId: id,
					},
				},
			}
		}

		shareRes, err := shareClient.ListOCMShares(ctx, shareRequest)
		if err != nil {
			return err
		}

		if shareRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(shareRes.Status)
		}

		if len(w) == 0 {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type",
				"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated"})

			for _, s := range shareRes.Shares {
				t.AppendRows([]table.Row{
					{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(),
						s.Grantee.Type.String(), s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId,
						time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)},
				})
			}
			t.Render()
		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(shareRes.Shares); err != nil {
				return err
			}
		}

		return nil
	}
	return cmd
}
