// Copyright 2018-2022 CERN
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
	"strings"
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

func transferCreateCommand() *command {
	cmd := newCommand("transfer-create")
	cmd.Description = func() string { return "create transfer between 2 sites" }
	cmd.Usage = func() string { return "Usage: transfer-create [-flags] <path>" }
	grantee := cmd.String("grantee", "", "the grantee, receiver of the transfer")
	granteeType := cmd.String("granteeType", "user", "the grantee type, one of: user, group (defaults to user)")
	idp := cmd.String("idp", "", "the idp of the grantee")
	userType := cmd.String("user-type", "primary", "the type of user account, defaults to primary")

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

		u := &userpb.UserId{OpaqueId: *grantee, Idp: *idp, Type: utils.UserTypeMap(*userType)}

		// check if invitation has been accepted
		acceptedUserRes, err := client.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
			RemoteUserId: u,
		})
		if err != nil {
			return err
		}
		if acceptedUserRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(acceptedUserRes.Status)
		}

		// verify resource stats
		statReq := &provider.StatRequest{
			Ref: &provider.Reference{Path: fn},
		}
		statRes, err := client.Stat(ctx, statReq)
		if err != nil {
			return err
		}
		if statRes.Status.Code != rpc.Code_CODE_OK {
			return formatError(statRes.Status)
		}

		providerInfoResp, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
			Domain: *idp,
		})
		if err != nil {
			return err
		}

		resourcePermissions, pint, err := getOCMSharePerm(editorPermission)
		if err != nil {
			return err
		}

		gt := provider.GranteeType_GRANTEE_TYPE_USER
		if strings.ToLower(*granteeType) == "group" {
			gt = provider.GranteeType_GRANTEE_TYPE_GROUP
		}

		createShareReq := &ocm.CreateOCMShareRequest{
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"permissions": {
						Decoder: "plain",
						Value:   []byte(strconv.Itoa(pint)),
					},
					"name": {
						Decoder: "plain",
						Value:   []byte(statRes.Info.Path),
					},
					"protocol": {
						Decoder: "plain",
						Value:   []byte("datatx"),
					},
				},
			},
			ResourceId: statRes.Info.Id,
			Grant: &ocm.ShareGrant{
				Grantee: &provider.Grantee{
					Type: gt,
					Id: &provider.Grantee_UserId{
						UserId: u,
					},
				},
				Permissions: resourcePermissions,
			},
			RecipientMeshProvider: providerInfoResp.ProviderInfo,
		}

		createShareResponse, err := client.CreateOCMShare(ctx, createShareReq)
		if err != nil {
			return err
		}
		if createShareResponse.Status.Code != rpc.Code_CODE_OK {
			if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return formatError(statRes.Status)
			}
			return err
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type", "Grantee.Idp", "Grantee.OpaqueId", "ShareType", "Created", "Updated"})

		s := createShareResponse.Share
		t.AppendRows([]table.Row{
			{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(),
				s.Grantee.Type.String(), s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId, s.ShareType.String(),
				time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)},
		})
		t.Render()
		return nil
	}

	return cmd
}
