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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			inv, err := b.OCM().CreateInvite(cmd.Context(), description)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), inv, func(w io.Writer) { renderInvites(w, []*invitepb.InviteToken{inv}) })
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			invites, err := b.OCM().Invites(cmd.Context())
			if err != nil {
				return err
			}
			if len(invites) == 0 {
				rt.Msg(cmd.ErrOrStderr(), "You have no open invitations.")
			}
			return rt.Print(cmd.OutOrStdout(), invites, func(w io.Writer) { renderInvites(w, invites) })
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if err := b.OCM().AcceptInvite(cmd.Context(), token, domain); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Accepted the invitation from %s. You can now share with them.", domain)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerDomain, "provider", "", "domain of the provider the invitation came from")
	return cmd
}

// splitInvite returns an empty domain for a plain token.
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			contacts, err := b.OCM().Contacts(cmd.Context(), filter)
			if err != nil {
				return err
			}
			if remove != "" {
				return removeContact(cmd, rt, b, contacts, remove)
			}
			if len(contacts) == 0 {
				rt.Msg(cmd.ErrOrStderr(), "No contacts yet. Start with 'reva ocm invite create'.")
			}
			return rt.Print(cmd.OutOrStdout(), contacts, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"Address", "OpaqueId", "Idp", "Mail", "DisplayName"})
				for _, u := range contacts {
					t.AppendRow(table.Row{u.Id.OpaqueId + "@" + u.Id.Idp, u.Id.OpaqueId, u.Id.Idp, u.Mail, u.DisplayName})
				}
				t.Render()
			})
		},
	}
	cmd.Flags().StringVar(&remove, "remove", "", "remove this contact, given as user@their-provider.org")
	return cmd
}

// The remote provider assigns the user id, which is often not the name in the
// address, so removeContact also matches the mail.
func removeContact(cmd *cobra.Command, rt *cli.Runtime, b backend.Backend, contacts []*userpb.User, address string) error {
	if !strings.Contains(address, "@") {
		return errors.New("--remove takes an address like user@their-provider.org")
	}
	i := slices.IndexFunc(contacts, func(u *userpb.User) bool {
		return strings.EqualFold(u.Id.OpaqueId+"@"+u.Id.Idp, address) || strings.EqualFold(u.Mail, address)
	})
	if i < 0 {
		return fmt.Errorf("no contact with the address %s: 'reva ocm contacts' lists them", address)
	}
	if err := b.OCM().RemoveContact(cmd.Context(), contacts[i].Id); err != nil {
		return err
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			providers, err := b.OCM().Providers(cmd.Context())
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), providers, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"Domain", "Name", "Homepage"})
				for _, p := range providers {
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

			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			share, err := b.OCM().CreateShare(cmd.Context(), pathRef(args[0]), &userpb.UserId{OpaqueId: grantee, Idp: idp}, methods)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), share, func(w io.Writer) { renderOCMShares(w, []*ocm.Share{share}) })
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
			var id *provider.ResourceId
			if resID != "" {
				var err error
				if id, err = parseResourceID(resID); err != nil {
					return err
				}
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			shares, err := b.OCM().Shares(cmd.Context(), id)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderOCMShares(w, shares) })
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			share, err := b.OCM().GetShare(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), share, func(w io.Writer) { fmt.Fprintln(w, protojson.Format(share)) })
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if err := b.OCM().UpdateShare(cmd.Context(), req); err != nil {
				return err
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if err := b.OCM().RemoveShare(cmd.Context(), args[0]); err != nil {
				return err
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			received, err := b.OCM().Received(cmd.Context())
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), received, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "RemoteShareId", "Type",
					"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated", "State", "ShareType"})
				for _, s := range received {
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			share, err := b.OCM().GetReceived(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), share, func(w io.Writer) { fmt.Fprintln(w, protojson.Format(share)) })
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if err := b.OCM().UpdateReceived(cmd.Context(), args[0], s); err != nil {
				return err
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
	return completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
		shares, err := b.OCM().Shares(ctx, nil)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(shares))
		for _, s := range shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.Grantee.GetUserId().GetOpaqueId())
		}
		return ids, nil
	})
}

func completeReceivedOCMShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
		received, err := b.OCM().Received(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(received))
		for _, s := range received {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.State.String())
		}
		return ids, nil
	})
}
