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
	"io"
	"os"
	"strconv"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/pkg/errors"
)

func ocmShareCreateCommand() *command {
	cmd := newCommand("ocm-share-create")
	cmd.Description = func() string { return "create OCM share to a user or group" }
	cmd.Usage = func() string { return "Usage: ocm-share-create [-flags] <path>" }
	grantType := cmd.String("type", "user", "grantee type (user or group)")
	grantee := cmd.String("grantee", "", "the grantee")
	idp := cmd.String("idp", "", "the idp of the grantee, default to same idp as the user triggering the action")
	userType := cmd.String("user-type", "primary", "the type of user account, defaults to primary")
	rol := cmd.String("rol", "viewer", "the permission for the share (viewer or editor)")

	cmd.ResetFlags = func() {
		*grantType, *grantee, *idp, *rol, *userType = "user", "", "", "viewer", "primary"
	}

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		// validate flags
		if *grantee == "" {
			return errors.New("Grantee cannot be empty: use -grantee flag\n" + cmd.Usage())
		}

		if *idp == "" {
			return errors.New("IdP cannot be empty: use -idp flag\n" + cmd.Usage())
		}

		fn := cmd.Args()[0]

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
			Domain: *idp,
		})
		if err != nil {
			return err
		}

		u := &userpb.UserId{OpaqueId: *grantee, Idp: *idp, Type: utils.UserTypeMap(*userType)}
		remoteUserRes, err := client.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
			RemoteUserId: u,
		})
		if err != nil {
			return err
		}
		if remoteUserRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(remoteUserRes.Status)
		}

		ref := &provider.Reference{Path: fn}
		req := &provider.StatRequest{Ref: ref}
		res, err := client.Stat(ctx, req)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		perm, pint, err := getOCMSharePerm(*rol)
		if err != nil {
			return err
		}

		gt := getGrantType(*grantType)
		grant := &ocm.ShareGrant{
			Permissions: perm,
			Grantee: &provider.Grantee{
				Type: gt,
				// For now, we only support user shares.
				// TODO (ishank011): To be updated once this is decided.
				Id: &provider.Grantee_UserId{UserId: u},
			},
		}

		opaqueObj := &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"permissions": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(strconv.Itoa(pint)),
				},
				"name": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(res.Info.Path),
				},
			},
		}

		shareRequest := &ocm.CreateOCMShareRequest{
			Opaque:                opaqueObj,
			ResourceId:            res.Info.Id,
			Grant:                 grant,
			RecipientMeshProvider: providerInfo.ProviderInfo,
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
			{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(),
				s.Grantee.Type.String(), s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId,
				time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)},
		})
		t.Render()

		return nil
	}
	return cmd
}

func getOCMSharePerm(p string) (*ocm.SharePermissions, int, error) {
	if p == viewerPermission {
		return &ocm.SharePermissions{
			Permissions: &provider.ResourcePermissions{
				GetPath:              true,
				InitiateFileDownload: true,
				ListFileVersions:     true,
				ListContainer:        true,
				Stat:                 true,
			},
		}, 1, nil
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
		}, 15, nil
	}
	return nil, 0, errors.New("invalid rol: " + p)
}
