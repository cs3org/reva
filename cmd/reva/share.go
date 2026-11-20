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

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	"github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func shareCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "share", Short: "Manage shares with users and groups"}
	received := &cobra.Command{Use: "received", Short: "Manage the shares you received"}
	received.AddCommand(shareReceivedListCommand(rt), shareReceivedUpdateCommand(rt))
	cmd.AddCommand(shareCreateCommand(rt), shareListCommand(rt), shareUpdateCommand(rt), shareRemoveCommand(rt), received)
	return cmd
}

func shareCreateCommand(rt *cli.Runtime) *cobra.Command {
	var grantType, grantee, idp, role, userType string
	cmd := &cobra.Command{
		Use:               "create <path>",
		Short:             "Share a file or folder with a user or a group",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			grant := &collaboration.ShareGrant{
				Permissions: &collaboration.SharePermissions{Permissions: perm},
				Grantee:     &provider.Grantee{},
			}
			switch grantType {
			case "user":
				grant.Grantee.Type = provider.GranteeType_GRANTEE_TYPE_USER
				grant.Grantee.Id = &provider.Grantee_UserId{UserId: &userpb.UserId{Idp: idp, OpaqueId: grantee, Type: utils.UserTypeMap(userType)}}
			case "group":
				grant.Grantee.Type = provider.GranteeType_GRANTEE_TYPE_GROUP
				grant.Grantee.Id = &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{Idp: idp, OpaqueId: grantee}}
			default:
				return fmt.Errorf("unknown grantee type %q: use user or group", grantType)
			}

			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			shareRes, err := client.CreateShare(ctx, &collaboration.CreateShareRequest{ResourceInfo: res.Info, Grant: grant})
			if err != nil {
				return err
			}
			if shareRes.Status.Code != rpc.Code_CODE_OK {
				return formatError(shareRes.Status)
			}
			return rt.Print(cmd.OutOrStdout(), shareRes.Share, func(w io.Writer) {
				renderShares(w, []*collaboration.Share{shareRes.Share})
			})
		},
	}
	cmd.Flags().StringVar(&grantType, "type", "user", "grantee type: user or group")
	cmd.Flags().StringVar(&grantee, "grantee", "", "the user or group to share with")
	cmd.Flags().StringVar(&idp, "idp", "", "the idp of the grantee (default: the idp of the caller)")
	cmd.Flags().StringVar(&role, "role", "viewer", "role of the grantee: viewer, editor, collab or denied")
	cmd.Flags().StringVar(&userType, "user-type", "primary", "the account type of the grantee")
	_ = cmd.MarkFlagRequired("grantee")
	return cmd
}

func shareListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the shares you manage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			shares, err := listShares(ctx, client, resID)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderShares(w, shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the shares of this resource (storage_id:opaque_id)")
	return cmd
}

func shareUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:               "update <share_id>",
		Short:             "Change the role of a share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if role != "viewer" && role != "editor" {
				return fmt.Errorf("unknown role %q: use viewer or editor", role)
			}
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.UpdateShare(ctx, &collaboration.UpdateShareRequest{
				Ref: shareRef(args[0]),
				Field: &collaboration.UpdateShareRequest_UpdateField{
					Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
						Permissions: &collaboration.SharePermissions{Permissions: perm},
					},
				},
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "new role of the grantee: viewer or editor")
	return cmd
}

func shareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove <share_id>",
		Short:             "Remove a share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.RemoveShare(ctx, &collaboration.RemoveShareRequest{Ref: shareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
}

func shareReceivedListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the shares you received",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Shares, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type",
					"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated", "State"})
				for _, rs := range res.Shares {
					t.AppendRow(append(shareRow(rs.Share), rs.State.String()))
				}
				t.Render()
			})
		},
	}
}

func shareReceivedUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var state string
	cmd := &cobra.Command{
		Use:   "update <share_id>",
		Short: "Accept or reject a share you received",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
			res, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(res.Shares))
			for _, rs := range res.Shares {
				ids = append(ids, rs.Share.Id.OpaqueId+"\t"+rs.State.String())
			}
			return ids, nil
		}),
		RunE: func(cmd *cobra.Command, args []string) error {
			var s collaboration.ShareState
			switch state {
			case "pending":
				s = collaboration.ShareState_SHARE_STATE_PENDING
			case "accepted":
				s = collaboration.ShareState_SHARE_STATE_ACCEPTED
			case "rejected":
				s = collaboration.ShareState_SHARE_STATE_REJECTED
			default:
				return fmt.Errorf("unknown state %q: use pending, accepted or rejected", state)
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{Ref: shareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			res.Share.State = s
			updateRes, err := client.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
				Share:      res.Share,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
			})
			if err != nil {
				return err
			}
			if updateRes.Status.Code != rpc.Code_CODE_OK {
				return formatError(updateRes.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&state, "state", "pending", "new state: pending, accepted or rejected")
	return cmd
}

func publicShareCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "public-share", Short: "Manage public links"}
	cmd.AddCommand(publicShareCreateCommand(rt), publicShareListCommand(rt), publicShareUpdateCommand(rt), publicShareRemoveCommand(rt))
	return cmd
}

func publicShareCreateCommand(rt *cli.Runtime) *cobra.Command {
	var role, description string
	var internal bool
	cmd := &cobra.Command{
		Use:               "create <path>",
		Short:             "Create a public link to a file or folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			shareRes, err := client.CreatePublicShare(ctx, &link.CreatePublicShareRequest{
				ResourceInfo: res.Info,
				Grant:        &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: perm}},
				Description:  description,
				Internal:     internal,
			})
			if err != nil {
				return err
			}
			if shareRes.Status.Code != rpc.Code_CODE_OK {
				return formatError(shareRes.Status)
			}
			return rt.Print(cmd.OutOrStdout(), shareRes.Share, func(w io.Writer) {
				renderPublicShares(w, []*link.PublicShare{shareRes.Share})
			})
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "role of the link: viewer or editor")
	cmd.Flags().StringVar(&description, "description", "", "description of the link")
	cmd.Flags().BoolVar(&internal, "internal", false, "mark the link as internal")
	return cmd
}

func publicShareListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the public links you created",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			shares, err := listPublicShares(ctx, client, resID)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderPublicShares(w, shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the links of this resource (storage_id:opaque_id)")
	return cmd
}

func publicShareUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role, description string
	cmd := &cobra.Command{
		Use:               "update <share_id>",
		Short:             "Change the role or the description of a public link",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if role != "viewer" && role != "editor" {
				return fmt.Errorf("unknown role %q: use viewer or editor", role)
			}
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			updates := []*link.UpdatePublicShareRequest_Update{{
				Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
				Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: perm}},
			}}
			if description != "" {
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:        link.UpdatePublicShareRequest_Update_TYPE_DESCRIPTION,
					Description: description,
				})
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			for _, u := range updates {
				res, err := client.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: publicShareRef(args[0]), Update: u})
				if err != nil {
					return err
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					return formatError(res.Status)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "new role of the link: viewer or editor")
	cmd.Flags().StringVar(&description, "description", "", "new description of the link")
	return cmd
}

func publicShareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove <share_id>",
		Short:             "Remove a public link",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.RemovePublicShare(ctx, &link.RemovePublicShareRequest{Ref: publicShareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
}

// listShares lists the shares of a resource, or else the shares that the caller created.
// The gateway rejects a listing without one of these filters.
func listShares(ctx context.Context, client gateway.GatewayAPIClient, resID string) ([]*collaboration.Share, error) {
	req := &collaboration.ListSharesRequest{}
	if resID != "" {
		id, err := parseResourceID(resID)
		if err != nil {
			return nil, err
		}
		req.Filters = []*collaboration.Filter{share.ResourceIDFilter(id)}
	} else {
		token, _ := appctx.ContextGetToken(ctx)
		who, err := client.WhoAmI(ctx, &gateway.WhoAmIRequest{Token: token})
		if err != nil {
			return nil, err
		}
		if who.Status.Code != rpc.Code_CODE_OK {
			return nil, formatError(who.Status)
		}
		req.Filters = []*collaboration.Filter{{
			Type: collaboration.Filter_TYPE_CREATOR,
			Term: &collaboration.Filter_Creator{Creator: who.User.Id},
		}}
	}
	res, err := client.ListShares(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(res.Status)
	}
	return res.Shares, nil
}

// listPublicShares lists the links that the caller created. The creator
// filter needs the user id, so it asks the gateway who the token belongs to.
func listPublicShares(ctx context.Context, client gateway.GatewayAPIClient, resID string) ([]*link.PublicShare, error) {
	token, _ := appctx.ContextGetToken(ctx)
	who, err := client.WhoAmI(ctx, &gateway.WhoAmIRequest{Token: token})
	if err != nil {
		return nil, err
	}
	if who.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(who.Status)
	}
	filters := []*link.ListPublicSharesRequest_Filter{{
		Type: link.ListPublicSharesRequest_Filter_TYPE_CREATOR,
		Term: &link.ListPublicSharesRequest_Filter_Creator{Creator: who.User.Id},
	}}
	if resID != "" {
		id, err := parseResourceID(resID)
		if err != nil {
			return nil, err
		}
		filters = append(filters, publicshare.ResourceIDFilter(id))
	}
	res, err := client.ListPublicShares(ctx, &link.ListPublicSharesRequest{Filters: filters})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, formatError(res.Status)
	}
	return res.Share, nil
}

func completeShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
		shares, err := listShares(ctx, client, "")
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(shares))
		for _, s := range shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.Grantee.GetUserId().GetOpaqueId()+s.Grantee.GetGroupId().GetOpaqueId())
		}
		return ids, nil
	})
}

func completePublicShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
		shares, err := listPublicShares(ctx, client, "")
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(shares))
		for _, s := range shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.Token)
		}
		return ids, nil
	})
}

// completeIDs completes the first argument with the ids that list returns.
// An id can have a description after a tab.
func completeIDs(rt *cli.Runtime, list func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error)) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if len(args) > 0 {
			return nil, none
		}
		// Completion does not run the PersistentPreRunE, so load the settings here.
		if err := rt.Load(cmd.Flags()); err != nil {
			return nil, none
		}
		client, ctx, err := rt.Gateway(cmd.Context())
		if err != nil {
			return nil, none
		}
		ids, err := list(ctx, client)
		if err != nil {
			return nil, none
		}
		return ids, none
	}
}

func getSharePerm(role string) (*provider.ResourcePermissions, error) {
	switch role {
	case "viewer":
		return permissions.NewViewerRole().CS3ResourcePermissions(), nil
	case "editor":
		return permissions.NewEditorRole().CS3ResourcePermissions(), nil
	case "collab":
		return permissions.NewManagerRole().CS3ResourcePermissions(), nil
	case "denied":
		return &provider.ResourcePermissions{}, nil
	default:
		return nil, fmt.Errorf("unknown role %q", role)
	}
}

func parseResourceID(s string) (*provider.ResourceId, error) {
	storageID, opaqueID, ok := strings.Cut(s, ":")
	if !ok || strings.Contains(opaqueID, ":") {
		return nil, fmt.Errorf("invalid resource id %q: use storage_id:opaque_id", s)
	}
	return &provider.ResourceId{StorageId: storageID, OpaqueId: opaqueID}, nil
}

func shareRef(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: id}}}
}

func publicShareRef(id string) *link.PublicShareReference {
	return &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}}}
}

func renderShares(w io.Writer, shares []*collaboration.Share) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type",
		"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated"})
	for _, s := range shares {
		t.AppendRow(shareRow(s))
	}
	t.Render()
}

func shareRow(s *collaboration.Share) table.Row {
	var idp, opaque string
	switch s.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		idp, opaque = s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		idp, opaque = s.Grantee.GetGroupId().Idp, s.Grantee.GetGroupId().OpaqueId
	}
	return table.Row{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(),
		s.Grantee.Type.String(), idp, opaque, time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)}
}

func renderPublicShares(w io.Writer, shares []*link.PublicShare) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Token", "Expiration", "Created", "Updated", "Description"})
	for _, s := range shares {
		t.AppendRow(table.Row{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(), s.Permissions.String(), s.Token,
			s.Expiration.String(), time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0), s.Description})
	}
	t.Render()
}
