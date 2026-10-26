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
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func ocmCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ocm",
		Short: "Share with people on another server",
		Long: "Share with people whose files are on another server, with Open Cloud Mesh.\n\n" +
			"It takes two steps. First you exchange an invitation, then you share.\n" +
			"Start with 'ocm invite create'. 'ocm contacts' shows who you can already\n" +
			"share with, and 'ocm share create' shares with them.",
	}

	invite := &cobra.Command{Use: "invite", Short: "Create, list and accept sharing invitations"}
	invite.AddCommand(ocmInviteCreateCommand(rt), ocmInviteListCommand(rt), ocmInviteAcceptCommand(rt))

	received := ocmReceivedCommand(rt)
	received.AddCommand(ocmShareReceivedGetCommand(rt), ocmShareReceivedUpdateCommand(rt))

	shares := &cobra.Command{Use: "share", Short: "Manage OCM shares"}
	shares.AddCommand(ocmShareCreateCommand(rt), ocmShareListCommand(rt), ocmShareGetCommand(rt),
		ocmShareUpdateCommand(rt), ocmShareRemoveCommand(rt))

	cmd.AddCommand(invite, ocmContactsCommand(rt), ocmProvidersCommand(rt), received, shares, transferCommand(rt))
	return cmd
}

func ocmInviteCreateCommand(rt *cli.Runtime) *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an invitation to send to someone",
		Long: "Create an invitation. Send the token to the person you want to share\n" +
			"with. They accept it on their own provider.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{Description: description})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.InviteToken, func(w io.Writer) { renderInvites(w, []*invitepb.InviteToken{res.InviteToken}) })
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "note to remind you who this invitation was for")
	return cmd
}

func ocmInviteListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the invitations you have created",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListInviteTokens(ctx, &invitepb.ListInviteTokensRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			if len(res.InviteTokens) == 0 {
				rt.Msg(cmd.ErrOrStderr(), "You have no open invitations.")
			}
			return rt.Print(cmd.OutOrStdout(), res.InviteTokens, func(w io.Writer) { renderInvites(w, res.InviteTokens) })
		},
	}
}

func renderInvites(w io.Writer, invites []*invitepb.InviteToken) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{"Token", "Description", "Expires"})
	for _, inv := range invites {
		exp := "-"
		if inv.Expiration != nil {
			exp = time.Unix(int64(inv.Expiration.Seconds), 0).Format(time.RFC3339)
		}
		t.AppendRow(table.Row{inv.Token, inv.Description, exp})
	}
	t.Render()
}

func ocmInviteAcceptCommand(rt *cli.Runtime) *cobra.Command {
	var providerDomain string
	cmd := &cobra.Command{
		Use:   "accept TOKEN",
		Short: "Accept an invitation you were sent",
		Long: "Accept an invitation.\n\n" +
			"You need the token and the provider it came from. If you were sent a link,\n" +
			"pass the link instead and the CLI reads both from it.",
		Example: "  reva ocm invite accept abc123 --provider other-lab.org\n" +
			"  reva ocm invite accept 'https://other-lab.org/ocm/invite?token=abc123'",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, domain := splitInvite(args[0])
			if token == "" {
				return errors.New("the invitation link has no token")
			}
			if providerDomain != "" {
				domain = providerDomain
			}
			if domain == "" {
				return errors.New("pass --provider with the domain the invitation came from, or give the whole invitation link instead of just the token")
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{Domain: domain})
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
			rt.Msg(cmd.ErrOrStderr(), "Accepted the invitation from %s. You can now share with them.", domain)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerDomain, "provider", "", "domain of the provider the invitation came from")
	return cmd
}

// splitInvite reads the token and the provider domain from an invitation
// link. A plain token gives an empty domain.
func splitInvite(arg string) (token, domain string) {
	u, err := url.Parse(arg)
	if err != nil || u.Host == "" {
		return arg, ""
	}
	q := u.Query()
	domain = u.Host
	if d := cmp.Or(q.Get("providerDomain"), q.Get("provider")); d != "" {
		domain = d
	}
	return q.Get("token"), domain
}

func ocmContactsCommand(rt *cli.Runtime) *cobra.Command {
	var remove string
	cmd := &cobra.Command{
		Use:     "contacts [FILTER]",
		Aliases: []string{"users"},
		Short:   "List the people you can share with",
		Long: "List the remote users that accepted your invitations, or that you accepted\n" +
			"an invitation from. --remove takes one off the list.",
		Args: cobra.MaximumNArgs(1),
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
			if remove != "" {
				return removeContact(cmd, rt, client, ctx, res.AcceptedUsers, remove)
			}
			if len(res.AcceptedUsers) == 0 {
				rt.Msg(cmd.ErrOrStderr(), "No contacts yet. Start with 'reva ocm invite create'.")
			}
			return rt.Print(cmd.OutOrStdout(), res.AcceptedUsers, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"Address", "OpaqueId", "Idp", "Mail", "DisplayName"})
				for _, u := range res.AcceptedUsers {
					t.AppendRow(table.Row{u.Id.OpaqueId + "@" + u.Id.Idp, u.Id.OpaqueId, u.Id.Idp, u.Mail, u.DisplayName})
				}
				t.Render()
			})
		},
	}
	cmd.Flags().StringVar(&remove, "remove", "", "remove this contact, given as user@their-provider.org")
	return cmd
}

// removeContact removes the contact with the address that a person types.
// The remote provider assigns the user id, which is often not the name in
// the address, so it also matches the mail.
func removeContact(cmd *cobra.Command, rt *cli.Runtime, client gateway.GatewayAPIClient, ctx context.Context, contacts []*userpb.User, address string) error {
	if !strings.Contains(address, "@") {
		return errors.New("--remove takes an address like user@their-provider.org")
	}
	i := slices.IndexFunc(contacts, func(u *userpb.User) bool {
		return strings.EqualFold(u.Id.OpaqueId+"@"+u.Id.Idp, address) || strings.EqualFold(u.Mail, address)
	})
	if i < 0 {
		return fmt.Errorf("no contact with the address %s: 'reva ocm contacts' lists them", address)
	}
	res, err := client.DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{RemoteUserId: contacts[i].Id})
	if err != nil {
		return err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return formatError(res.Status)
	}
	rt.Msg(cmd.ErrOrStderr(), "Removed %s.", address)
	return nil
}

func ocmProvidersCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List the other servers you can share with",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListAllProviders(ctx, &ocmprovider.ListAllProvidersRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Providers, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"Domain", "Name", "Homepage"})
				for _, p := range res.Providers {
					t.AppendRow(table.Row{p.Domain, cmp.Or(p.FullName, p.Name), p.Homepage})
				}
				t.Render()
			})
		},
	}
}

func ocmShareCreateCommand(rt *cli.Runtime) *cobra.Command {
	var grantee, idp, role string
	var webdav, webapp bool
	cmd := &cobra.Command{
		Use:               "create PATH",
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
		Use:               "get SHARE_ID",
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
		Use:               "update SHARE_ID",
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
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&webdavRole, "webdav-role", "", "new role for WebDAV access: viewer or editor")
	cmd.Flags().StringVar(&webappRole, "webapp-role", "", "new role for app access: viewer or editor")
	return cmd
}

func ocmShareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove SHARE_ID",
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
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
}

func ocmReceivedCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "received",
		Short: "List shares you have received from another server",
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
		Use:               "get SHARE_ID",
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
		Use:               "update SHARE_ID",
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
			rt.Msg(cmd.ErrOrStderr(), "OK")
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
