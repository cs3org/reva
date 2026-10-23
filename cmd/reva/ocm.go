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
	"errors"
	"fmt"
	"io"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func ocmCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "ocm", Short: "Manage Open Cloud Mesh invites, users and shares"}

	invite := &cobra.Command{Use: "invite", Short: "Manage OCM invites"}
	invite.AddCommand(ocmInviteGenerateCommand(rt), ocmInviteForwardCommand(rt))

	users := &cobra.Command{Use: "user", Short: "Manage the remote users that accepted your invites"}
	users.AddCommand(ocmUserFindCommand(rt), ocmUserRemoveCommand(rt))

	shares := &cobra.Command{Use: "share", Short: "Manage OCM shares"}
	received := &cobra.Command{Use: "received", Short: "Manage the OCM shares you received"}
	received.AddCommand(ocmShareReceivedListCommand(rt), ocmShareReceivedGetCommand(rt), ocmShareReceivedUpdateCommand(rt))
	shares.AddCommand(ocmShareCreateCommand(rt), ocmShareListCommand(rt), ocmShareGetCommand(rt),
		ocmShareUpdateCommand(rt), ocmShareRemoveCommand(rt), received)

	cmd.AddCommand(invite, users, shares)
	return cmd
}

func ocmInviteGenerateCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate an invite token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.InviteToken, func(w io.Writer) { fmt.Fprintln(w, res.InviteToken) })
		},
	}
}

func ocmInviteForwardCommand(rt *cli.Runtime) *cobra.Command {
	var token, idp string
	cmd := &cobra.Command{
		Use:   "forward",
		Short: "Accept an invite token from another provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{Domain: idp})
			if err != nil {
				return err
			}
			if providerInfo.Status.Code != rpc.Code_CODE_OK {
				return formatError(providerInfo.Status)
			}
			res, err := client.ForwardInvite(ctx, &invitepb.ForwardInviteRequest{
				InviteToken:          &invitepb.InviteToken{Token: token},
				OriginSystemProvider: providerInfo.ProviderInfo,
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "the invite token")
	cmd.Flags().StringVar(&idp, "idp", "", "the domain of the provider that generated the token")
	_ = cmd.MarkFlagRequired("token")
	_ = cmd.MarkFlagRequired("idp")
	return cmd
}

func ocmUserFindCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "find [filter]",
		Short: "Find the remote users that accepted your invites",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter string
			if len(args) == 1 {
				filter = args[0]
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.FindAcceptedUsers(ctx, &invitepb.FindAcceptedUsersRequest{Filter: filter})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.AcceptedUsers, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"OpaqueId", "Idp", "Mail", "DisplayName"})
				for _, u := range res.AcceptedUsers {
					t.AppendRow(table.Row{u.Id.OpaqueId, u.Id.Idp, u.Mail, u.DisplayName})
				}
				t.Render()
			})
		},
	}
}

func ocmUserRemoveCommand(rt *cli.Runtime) *cobra.Command {
	var user, idp string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a remote user from your list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{
				RemoteUserId: &userpb.UserId{Type: userpb.UserType_USER_TYPE_FEDERATED, Idp: idp, OpaqueId: user},
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
	cmd.Flags().StringVar(&user, "user", "", "the id of the remote user")
	cmd.Flags().StringVar(&idp, "idp", "", "the idp of the remote user")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("idp")
	return cmd
}

func ocmShareCreateCommand(rt *cli.Runtime) *cobra.Command {
	var grantee, idp, role string
	var webdav, webapp bool
	cmd := &cobra.Command{
		Use:               "create <path>",
		Short:             "Share a file or folder with a remote user",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !webdav && !webapp {
				webdav = true
			}
			perm, err := getOCMSharePerm(role)
			if err != nil {
				return err
			}
			var methods []*ocm.AccessMethod
			if webdav {
				methods = append(methods, ocmshare.NewWebDavAccessMethod(perm, []ocm.AccessType{}, []string{}))
			}
			if webapp {
				methods = append(methods, ocmshare.NewWebappAccessMethod(perm, ocmshare.DefaultWebappRequirements, ""))
			}

			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{Domain: idp})
			if err != nil {
				return err
			}
			if providerInfo.Status.Code != rpc.Code_CODE_OK {
				return formatError(providerInfo.Status)
			}
			token, _ := appctx.ContextGetToken(ctx)
			who, err := client.WhoAmI(ctx, &gateway.WhoAmIRequest{Token: token})
			if err != nil {
				return err
			}
			if who.Status.Code != rpc.Code_CODE_OK {
				return formatError(who.Status)
			}
			userFilter, err := utils.MarshalProtoV1ToJSON(who.User.Id)
			if err != nil {
				return err
			}
			remoteUser, err := client.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
				RemoteUserId: &userpb.UserId{OpaqueId: grantee, Idp: idp, Type: userpb.UserType_USER_TYPE_FEDERATED},
				Opaque:       &types.Opaque{Map: map[string]*types.OpaqueEntry{"user-filter": {Decoder: "json", Value: userFilter}}},
			})
			if err != nil {
				return err
			}
			if remoteUser.Status.Code != rpc.Code_CODE_OK {
				return formatError(remoteUser.Status)
			}
			stat, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if stat.Status.Code != rpc.Code_CODE_OK {
				return formatError(stat.Status)
			}
			res, err := client.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
				ResourceId: stat.Info.Id,
				// Only user grantees are supported for now.
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: remoteUser.RemoteUser.Id},
				},
				RecipientMeshProvider: providerInfo.ProviderInfo,
				AccessMethods:         methods,
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Share, func(w io.Writer) { renderOCMShares(w, []*ocm.Share{res.Share}) })
		},
	}
	cmd.Flags().StringVar(&grantee, "grantee", "", "the id of the remote user")
	cmd.Flags().StringVar(&idp, "idp", "", "the idp of the remote user")
	cmd.Flags().StringVar(&role, "role", "viewer", "role of the grantee for webdav and webapp access: viewer or editor")
	cmd.Flags().BoolVar(&webdav, "webdav", false, "give WebDAV access (the default when no access is given)")
	cmd.Flags().BoolVar(&webapp, "webapp", false, "give access through apps")
	_ = cmd.MarkFlagRequired("grantee")
	_ = cmd.MarkFlagRequired("idp")
	return cmd
}

func ocmShareListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the OCM shares you manage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &ocm.ListOCMSharesRequest{}
			if resID != "" {
				id, err := parseResourceID(resID)
				if err != nil {
					return err
				}
				req.Filters = []*ocm.ListOCMSharesRequest_Filter{ocmshare.ResourceIDFilter(id)}
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListOCMShares(ctx, req)
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Shares, func(w io.Writer) { renderOCMShares(w, res.Shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the shares of this resource (storage_id:opaque_id)")
	return cmd
}

func ocmShareGetCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "get <share_id>",
		Short:             "Show an OCM share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeOCMShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GetOCMShare(ctx, &ocm.GetOCMShareRequest{Ref: ocmShareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Share, func(w io.Writer) { fmt.Fprintln(w, protojson.Format(res.Share)) })
		},
	}
}

func ocmShareUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var webdavRole, webappRole string
	cmd := &cobra.Command{
		Use:               "update <share_id>",
		Short:             "Change the access of an OCM share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeOCMShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if webdavRole == "" && webappRole == "" {
				return errors.New("use at least one of --webdav-role and --webapp-role")
			}
			req := &ocm.UpdateOCMShareRequest{Ref: ocmShareRef(args[0])}
			if webdavRole != "" {
				perm, err := getOCMSharePerm(webdavRole)
				if err != nil {
					return err
				}
				req.Field = append(req.Field, &ocm.UpdateOCMShareRequest_UpdateField{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: &ocm.AccessMethod{Term: &ocm.AccessMethod_WebdavOptions{
							WebdavOptions: &ocm.WebDAVAccessMethod{Permissions: perm, Requirements: []string{}},
						}},
					},
				})
			}
			if webappRole != "" {
				perm, err := getOCMSharePerm(webappRole)
				if err != nil {
					return err
				}
				req.Field = append(req.Field, &ocm.UpdateOCMShareRequest_UpdateField{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: &ocm.AccessMethod{Term: &ocm.AccessMethod_WebappOptions{
							WebappOptions: &ocm.WebappAccessMethod{Permissions: perm},
						}},
					},
				})
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.UpdateOCMShare(ctx, req)
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
	cmd.Flags().StringVar(&webdavRole, "webdav-role", "", "new role for WebDAV access: viewer or editor")
	cmd.Flags().StringVar(&webappRole, "webapp-role", "", "new role for app access: viewer or editor")
	return cmd
}

func ocmShareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove <share_id>",
		Short:             "Remove an OCM share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeOCMShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.RemoveOCMShare(ctx, &ocm.RemoveOCMShareRequest{Ref: ocmShareRef(args[0])})
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

func ocmShareReceivedListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the OCM shares you received",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListReceivedOCMShares(ctx, &ocm.ListReceivedOCMSharesRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Shares, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "RemoteShareId", "Type",
					"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated", "State", "ShareType"})
				for _, s := range res.Shares {
					t.AppendRow(table.Row{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.RemoteShareId,
						s.Grantee.Type.String(), s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId,
						time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0), s.State.String(), s.ShareType.String()})
				}
				t.Render()
			})
		},
	}
}

func ocmShareReceivedGetCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "get <share_id>",
		Short:             "Show an OCM share you received",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeReceivedOCMShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GetReceivedOCMShare(ctx, &ocm.GetReceivedOCMShareRequest{Ref: ocmShareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Share, func(w io.Writer) { fmt.Fprintln(w, protojson.Format(res.Share)) })
		},
	}
}

func ocmShareReceivedUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var state string
	cmd := &cobra.Command{
		Use:               "update <share_id>",
		Short:             "Accept or reject an OCM share you received",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeReceivedOCMShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			var s ocm.ShareState
			switch state {
			case "pending":
				s = ocm.ShareState_SHARE_STATE_PENDING
			case "accepted":
				s = ocm.ShareState_SHARE_STATE_ACCEPTED
			case "rejected":
				s = ocm.ShareState_SHARE_STATE_REJECTED
			default:
				return fmt.Errorf("unknown state %q: use pending, accepted or rejected", state)
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GetReceivedOCMShare(ctx, &ocm.GetReceivedOCMShareRequest{Ref: ocmShareRef(args[0])})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			res.Share.State = s
			updateRes, err := client.UpdateReceivedOCMShare(ctx, &ocm.UpdateReceivedOCMShareRequest{
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

func getOCMSharePerm(role string) (*provider.ResourcePermissions, error) {
	if role != "viewer" && role != "editor" {
		return nil, fmt.Errorf("unknown role %q: use viewer or editor", role)
	}
	return getSharePerm(role)
}

func ocmShareRef(id string) *ocm.ShareReference {
	return &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: id}}}
}

func renderOCMShares(w io.Writer, shares []*ocm.Share) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Type",
		"ShareType", "Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated"})
	for _, s := range shares {
		t.AppendRow(table.Row{s.Id.OpaqueId, s.Owner.Idp, s.Owner.OpaqueId, s.ResourceId.String(),
			s.Grantee.Type.String(), s.ShareType.String(), s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId,
			time.Unix(int64(s.Ctime.Seconds), 0), time.Unix(int64(s.Mtime.Seconds), 0)})
	}
	t.Render()
}

func completeOCMShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
		res, err := client.ListOCMShares(ctx, &ocm.ListOCMSharesRequest{})
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(res.Shares))
		for _, s := range res.Shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.Grantee.GetUserId().GetOpaqueId())
		}
		return ids, nil
	})
}

func completeReceivedOCMShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
		res, err := client.ListReceivedOCMShares(ctx, &ocm.ListReceivedOCMSharesRequest{})
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(res.Shares))
		for _, s := range res.Shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.State.String())
		}
		return ids, nil
	})
}
