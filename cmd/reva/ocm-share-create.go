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
	"fmt"
	"os"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/jedib0t/go-pretty/table"
	"github.com/pkg/errors"
)

func ocmShareCreateCommand() *command {
	cmd := newCommand("ocm-share-create")
	cmd.Description = func() string { return "create OCM share to a user or group" }
	cmd.Usage = func() string { return "Usage: ocm-share create [-flags] <path>" }
	grantType := cmd.String("type", "user", "grantee type (user or group)")
	grantee := cmd.String("grantee", "", "the grantee")
	idp := cmd.String("idp", "", "the idp of the grantee, default to same idp as the user triggering the action")
	rol := cmd.String("rol", "viewer", "the permission for the share (viewer or editor)")
	cmd.Action = func() error {
		if cmd.NArg() < 1 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		// validate flags
		if *grantee == "" {
			fmt.Println("grantee cannot be empty: use -grantee flag")
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		}

		req := &provider.StatRequest{Ref: ref}
		res, err := client.Stat(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		perm, err := getOCMSharePerm(*rol)
		if err != nil {
			return err
		}

		gt := getGrantType(*grantType)

		grant := &ocm.ShareGrant{
			Permissions: perm,
			Grantee: &provider.Grantee{
				Type: gt,
				Id: &userpb.UserId{
					Idp:      *idp,
					OpaqueId: *grantee,
				},
			},
		}
		shareRequest := &ocm.CreateOCMShareRequest{
			ResourceId: res.Info.Id,
			Grant:      grant,
		}

		shareRes, err := client.CreateOCMShare(ctx, shareRequest)
		if err != nil {
			return err
		}

		if shareRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(shareRes.Status)
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type", "Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated"})

		s := shareRes.Share
		t.AppendRows([]table.Row{
			{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(), s.Grantee.Type.String(), s.Grantee.Id.Idp, s.Grantee.Id.OpaqueId, time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)},
		})
		t.Render()

		return nil
	}
	return cmd
}

func getOCMSharePerm(p string) (*ocm.SharePermissions, error) {
	if p == viewerPermission {
		return &ocm.SharePermissions{
			Permissions: &provider.ResourcePermissions{
				GetPath:              true,
				InitiateFileDownload: true,
				ListFileVersions:     true,
				ListContainer:        true,
				Stat:                 true,
			},
		}, nil
	} else if p == editorPermission {
		return &ocm.SharePermissions{
			Permissions: &provider.ResourcePermissions{
				GetPath:              true,
				InitiateFileDownload: true,
				ListFileVersions:     true,
				ListContainer:        true,
				Stat:                 true,
				CreateContainer:      true,
				Delete:               true,
				InitiateFileUpload:   true,
				RestoreFileVersion:   true,
				Move:                 true,
			},
		}, nil
	}
	return nil, errors.New("invalid rol: " + p)
}
