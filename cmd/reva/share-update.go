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
	"fmt"
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/pkg/errors"
)

func shareUpdateCommand() *command {
	cmd := newCommand("share-update")
	cmd.Description = func() string { return "update a share" }
	cmd.Usage = func() string { return "Usage: share-update [-flags] <share_id>" }
	rol := cmd.String("rol", "viewer", "the permission for the share (viewer or editor)")

	cmd.ResetFlags = func() {
		*rol = "viewer"
	}
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		// validate flags
		if *rol != viewerPermission && *rol != editorPermission {
			return errors.New("Invalid rol: rol must be viewer or editor\n" + cmd.Usage())
		}

		id := cmd.Args()[0]

		ctx := getAuthContext()
		shareClient, err := getClient()
		if err != nil {
			return err
		}

		perm, err := getSharePerm(*rol)
		if err != nil {
			return err
		}

		shareRequest := &collaboration.UpdateShareRequest{
			Ref: &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: id,
					},
				},
			},
			Field: &collaboration.UpdateShareRequest_UpdateField{
				Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
					Permissions: &collaboration.SharePermissions{
						Permissions: perm,
					},
				},
			},
		}

		shareRes, err := shareClient.UpdateShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if shareRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(shareRes.Status)
		}

		fmt.Println("OK")
		return nil
	}
	return cmd
}
