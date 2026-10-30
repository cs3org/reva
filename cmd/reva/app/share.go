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
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func shareCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "share", Short: "Manage shares with users and groups"}
	received := &cobra.Command{Use: "received", Short: "Manage the shares you received"}
	received.AddCommand(shareReceivedListCommand(rt), shareReceivedUpdateCommand(rt))
	cmd.AddCommand(shareCreateCommand(rt), shareListCommand(rt), shareGetCommand(rt), shareUpdateCommand(rt), shareRemoveCommand(rt), received,
		shareSyncExpiryCommand(rt), shareTransferCommand(rt), shareMarkOrphanCommand(rt), shareMoveCommand(rt))
	return cmd
}

func shareFallback(rt *cli.Runtime, id string) (cli.Fallback, func(context.Context) (*collaboration.Share, error)) {
	return cli.Fallback{Gate: rt, What: "share " + id}, func(ctx context.Context) (*collaboration.Share, error) {
		sa, err := rt.SharesAdmin(ctx)
		if err != nil {
			return nil, err
		}
		return sa.GetShare(ctx, shareRef(id))
	}
}

func shareGetCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "get SHARE_ID",
		Short:             "Show a share",
		Long:              "Show a share.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := shareFallback(rt, args[0])
			s, err := cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*collaboration.Share, error) {
				return b.Shares().Get(ctx, args[0])
			}, lookup, nil)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), s, func(w io.Writer) { renderShares(w, []*collaboration.Share{s}) })
		},
	}
}

func shareCreateCommand(rt *cli.Runtime) *cobra.Command {
	var users, groups []string
	var idp, role, userType string
	var ignoreExisting bool
	cmd := &cobra.Command{
		Use:               "create PATH",
		Short:             "Share a file or folder with users and groups",
		Example:           "  reva share create /home/Documents --with marie --role editor",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(users) == 0 && len(groups) == 0 {
				return errors.New("pass --with USER or --with-group GROUP")
			}
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			var grantees []*provider.Grantee
			for _, u := range users {
				grantees = append(grantees, &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{Idp: idp, OpaqueId: u, Type: utils.UserTypeMap(userType)}},
				})
			}
			for _, g := range groups {
				grantees = append(grantees, &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
					Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{Idp: idp, OpaqueId: g}},
				})
			}

			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			info, err := b.Files().Stat(ctx, pathRef(args[0]))
			if err != nil {
				return err
			}
			shares := make([]*collaboration.Share, 0, len(grantees))
			for _, g := range grantees {
				s, err := b.Shares().Create(ctx, &collaboration.CreateShareRequest{
					ResourceInfo: info,
					Grant:        &collaboration.ShareGrant{Permissions: &collaboration.SharePermissions{Permissions: perm}, Grantee: g},
				})
				if ignoreExisting && isAlreadyExists(err) {
					rt.Msg(cmd.ErrOrStderr(), "%s already has a share on %s", granteeName(g), args[0])
					if s, err = existingShare(ctx, b, info.Id, g); err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
				if s != nil {
					shares = append(shares, s)
				}
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderShares(w, shares) })
		},
	}
	cmd.Flags().StringArrayVar(&users, "with", nil, "user name to share with (repeatable)")
	cmd.Flags().StringArrayVar(&groups, "with-group", nil, "group to share with (repeatable)")
	cmd.Flags().StringVar(&idp, "idp", "", "the idp of the grantees (default: the idp of the caller)")
	cmd.Flags().StringVar(&role, "role", "viewer", "viewer, editor, manager, or denied")
	cmd.Flags().StringVar(&userType, "user-type", "primary", "the account type of the users")
	cmd.Flags().BoolVar(&ignoreExisting, "ignore-existing", false, "a share that exists already is not an error")
	return cmd
}

func shareListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	var op shareListOptions
	cmd := &cobra.Command{
		Use:               "list [PATH]",
		Short:             "List the shares on a path, or all the shares you manage",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rt.Plane() == cli.AdminPlane {
				return adminListShares(cmd, rt, args, resID, op)
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			id, err := resourceFilter(cmd.Context(), b, args, resID)
			if err != nil {
				return err
			}
			shares, err := b.Shares().List(cmd.Context(), id)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderShares(w, shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the shares of this resource (storage_id:opaque_id)")
	op.addFlags(cmd, "shares", true)
	return cmd
}

// resourceFilter returns nil when there is no filter.
func resourceFilter(ctx context.Context, b backend.Backend, args []string, resID string) (*provider.ResourceId, error) {
	switch {
	case len(args) == 1 && resID != "":
		return nil, errors.New("pass either PATH or --by-resource-id, not both")
	case resID != "":
		return parseResourceID(resID)
	case len(args) == 1:
		info, err := b.Files().Stat(ctx, pathRef(args[0]))
		if err != nil {
			return nil, err
		}
		return info.Id, nil
	}
	return nil, nil
}

func shareUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role, expires string
	cmd := &cobra.Command{
		Use:               "update SHARE_ID",
		Short:             "Change the role or the expiration of a share",
		Long:              "Change the role or the expiration of a share.\n\n" + cli.FallbackHelp,
		Example:           "  reva share update 4711 --role editor --expires 2026-12-31",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if role == "" && expires == "" {
				return errors.New("pass --role, --expires or both")
			}
			var fields []*collaboration.UpdateShareRequest_UpdateField
			if role != "" {
				if role != "viewer" && role != "editor" {
					return fmt.Errorf("unknown role %q: use viewer or editor", role)
				}
				perm, err := getSharePerm(role)
				if err != nil {
					return err
				}
				fields = append(fields, &collaboration.UpdateShareRequest_UpdateField{
					Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
						Permissions: &collaboration.SharePermissions{Permissions: perm},
					},
				})
			}
			if expires != "" {
				exp, err := admin.ParseExpiry(expires)
				if err != nil {
					return err
				}
				fields = append(fields, &collaboration.UpdateShareRequest_UpdateField{
					Field: &collaboration.UpdateShareRequest_UpdateField_Expiration{Expiration: exp},
				})
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := shareFallback(rt, args[0])
			_, err = cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*collaboration.Share, error) {
				var s *collaboration.Share
				for _, f := range fields {
					if s, err = b.Shares().Update(ctx, &collaboration.UpdateShareRequest{Ref: shareRef(args[0]), Field: f}); err != nil {
						return nil, err
					}
				}
				return s, nil
			}, lookup, func(ctx context.Context, _ *collaboration.Share) (*collaboration.Share, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return sa.UpdateShare(ctx, shareRef(args[0]), fields)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "new role of the grantee: viewer or editor")
	cmd.Flags().StringVar(&expires, "expires", "", "new expiration: a date (2006-01-02), an RFC 3339 time, or none")
	return cmd
}

func shareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	var dbOnly bool
	cmd := &cobra.Command{
		Use:               "remove SHARE_ID",
		Short:             "Remove a share",
		Long:              "Remove a share.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbOnly {
				if err := rt.Confirm(fmt.Sprintf("remove the record of share %s and keep its storage grant", args[0])); err != nil {
					return err
				}
				ctx, err := rt.AdminContext(cmd.Context())
				if err != nil {
					return err
				}
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return err
				}
				if err := sa.RemoveShare(ctx, shareRef(args[0]), true); err != nil {
					return err
				}
				rt.Msg(cmd.ErrOrStderr(), "Removed the record of share %s", args[0])
				return nil
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := shareFallback(rt, args[0])
			_, err = cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*collaboration.Share, error) {
				return nil, b.Shares().Remove(ctx, args[0])
			}, lookup, func(ctx context.Context, s *collaboration.Share) (*collaboration.Share, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return s, sa.RemoveShare(ctx, shareRef(args[0]), false)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Removed share %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&dbOnly, "db-only", false, "remove the record only, and keep the storage grant (for a share whose owner is gone)")
	cli.AdminFlag(cmd.Flags(), "db-only")
	return cmd
}

func shareReceivedListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the shares you received",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			received, err := b.Shares().Received(cmd.Context())
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), received, func(w io.Writer) {
				t := table.NewWriter()
				t.SetOutputMirror(w)
				t.AppendHeader(table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type",
					"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated", "State"})
				for _, rs := range received {
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
		Use:   "update SHARE_ID",
		Short: "Accept or reject a share you received",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
			received, err := b.Shares().Received(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(received))
			for _, rs := range received {
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
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if _, err := b.Shares().UpdateReceived(cmd.Context(), args[0], s); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&state, "state", "pending", "new state: pending, accepted or rejected")
	return cmd
}

func linkCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "link", Short: "Create and manage public links"}
	cmd.AddCommand(linkCreateCommand(rt), linkListCommand(rt), linkGetCommand(rt), linkUpdateCommand(rt), linkRemoveCommand(rt),
		linkTransferCommand(rt), linkMarkOrphanCommand(rt))
	return cmd
}

func linkFallback(rt *cli.Runtime, arg string, ref *link.PublicShareReference) (cli.Fallback, func(context.Context) (*link.PublicShare, error)) {
	return cli.Fallback{Gate: rt, What: "link " + arg}, func(ctx context.Context) (*link.PublicShare, error) {
		sa, err := rt.SharesAdmin(ctx)
		if err != nil {
			return nil, err
		}
		return sa.GetPublicLink(ctx, ref)
	}
}

func linkGetCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "get LINK_ID|TOKEN",
		Short:             "Show a public link",
		Long:              "Show a public link, by its id or by its token.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			refs := linkRefs(args[0])
			fb := cli.Fallback{Gate: rt, What: "link " + args[0]}
			s, err := cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*link.PublicShare, error) {
				return firstFound(refs, func(ref *link.PublicShareReference) (*link.PublicShare, error) {
					return b.Links().Get(ctx, ref)
				})
			}, func(ctx context.Context) (*link.PublicShare, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return firstFound(refs, func(ref *link.PublicShareReference) (*link.PublicShare, error) {
					return sa.GetPublicLink(ctx, ref)
				})
			}, nil)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), s, func(w io.Writer) { renderPublicShares(w, []*link.PublicShare{s}) })
		},
	}
}

// linkRefs treats a number only as an id, because the sql manager uses numbers
// as ids.
func linkRefs(arg string) []*link.PublicShareReference {
	refs := []*link.PublicShareReference{publicShareRef(arg)}
	if strings.Trim(arg, "0123456789") != "" {
		refs = append(refs, &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: arg}})
	}
	return refs
}

func firstFound(refs []*link.PublicShareReference, get func(*link.PublicShareReference) (*link.PublicShare, error)) (*link.PublicShare, error) {
	var err error
	for _, ref := range refs {
		var s *link.PublicShare
		if s, err = get(ref); err == nil || !(cli.IsNotFound(err) || status.Code(err) == codes.NotFound) {
			return s, err
		}
	}
	return nil, err
}

func linkCreateCommand(rt *cli.Runtime) *cobra.Command {
	var role, description string
	var internal bool
	cmd := &cobra.Command{
		Use:               "create PATH",
		Short:             "Create a public link",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			perm, err := getSharePerm(role)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			info, err := b.Files().Stat(ctx, pathRef(args[0]))
			if err != nil {
				return err
			}
			s, err := b.Links().Create(ctx, &link.CreatePublicShareRequest{
				ResourceInfo: info,
				Grant:        &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: perm}},
				Description:  description,
				Internal:     internal,
			})
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), s, func(w io.Writer) {
				renderPublicShares(w, []*link.PublicShare{s})
			})
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "role of the link: viewer or editor")
	cmd.Flags().StringVar(&description, "description", "", "description of the link")
	cmd.Flags().BoolVar(&internal, "internal", false, "mark the link as internal")
	return cmd
}

func linkListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	var op shareListOptions
	cmd := &cobra.Command{
		Use:               "list [PATH]",
		Short:             "List the public links on a path, or all the links you created",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rt.Plane() == cli.AdminPlane {
				return adminListLinks(cmd, rt, args, resID, op)
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			id, err := resourceFilter(cmd.Context(), b, args, resID)
			if err != nil {
				return err
			}
			shares, err := b.Links().List(cmd.Context(), id)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderPublicShares(w, shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the links of this resource (storage_id:opaque_id)")
	op.addFlags(cmd, "links", false)
	return cmd
}

func linkUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role, description, expires string
	var password, noPassword bool
	cmd := &cobra.Command{
		Use:   "update LINK_ID",
		Short: "Change the role, the expiration, the password or the description of a public link",
		Long: "Change the role, the expiration, the password or the description of a public link.\n" +
			"--password reads the new password from the terminal, or from one line of stdin.\n\n" + cli.FallbackHelp,
		Example:           "  reva link update 42 --expires 2026-12-31\n  echo \"$PW\" | reva link update 42 --password",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			if role == "" && description == "" && expires == "" && !password && !noPassword {
				return errors.New("pass --role, --expires, --password, --no-password or --description")
			}
			var updates []*link.UpdatePublicShareRequest_Update
			if role != "" {
				if role != "viewer" && role != "uploader" && role != "editor" {
					return fmt.Errorf("unknown role %q: use viewer, uploader or editor", role)
				}
				perm, err := getSharePerm(role)
				if err != nil {
					return err
				}
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
					Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: perm}},
				})
			}
			if expires != "" {
				exp, err := admin.ParseExpiry(expires)
				if err != nil {
					return err
				}
				// The link manager removes the expiration only for a nil timestamp.
				if exp.GetSeconds() == 0 {
					exp = nil
				}
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:  link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
					Grant: &link.Grant{Expiration: exp},
				})
			}
			if password || noPassword {
				var pw string
				if password {
					var err error
					if pw, err = cli.ReadSecret(cmd, "New password of the link: "); err != nil {
						return err
					}
					if pw == "" {
						return errors.New("the password is empty: use --no-password to remove the password")
					}
				}
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:  link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
					Grant: &link.Grant{Password: pw},
				})
			}
			if description != "" {
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:        link.UpdatePublicShareRequest_Update_TYPE_DESCRIPTION,
					Description: description,
				})
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			ref := publicShareRef(args[0])
			fb, lookup := linkFallback(rt, args[0], ref)
			_, err = cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*link.PublicShare, error) {
				var s *link.PublicShare
				for _, u := range updates {
					if s, err = b.Links().Update(ctx, &link.UpdatePublicShareRequest{Ref: ref, Update: u}); err != nil {
						return nil, err
					}
				}
				return s, nil
			}, lookup, func(ctx context.Context, s *link.PublicShare) (*link.PublicShare, error) {
				if description != "" {
					return nil, errors.New("admin rights cannot change the description of a link: --description works only on your own links")
				}
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return sa.UpdatePublicLink(ctx, ref, updates)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "new role of the link: viewer, uploader or editor")
	cmd.Flags().StringVar(&expires, "expires", "", "new expiration: a date (2006-01-02), an RFC 3339 time, or none")
	cmd.Flags().BoolVar(&password, "password", false, "set a new password, read from the terminal or from stdin")
	cmd.Flags().BoolVar(&noPassword, "no-password", false, "remove the password")
	cmd.Flags().StringVar(&description, "description", "", "new description of the link")
	cmd.MarkFlagsMutuallyExclusive("password", "no-password")
	return cmd
}

func linkRemoveCommand(rt *cli.Runtime) *cobra.Command {
	var dbOnly bool
	cmd := &cobra.Command{
		Use:               "remove LINK_ID",
		Short:             "Remove a public link",
		Long:              "Remove a public link.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := publicShareRef(args[0])
			if dbOnly {
				if err := rt.Confirm(fmt.Sprintf("remove the record of link %s without the checks of the gateway", args[0])); err != nil {
					return err
				}
				ctx, err := rt.AdminContext(cmd.Context())
				if err != nil {
					return err
				}
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return err
				}
				if err := sa.RemovePublicLink(ctx, ref, true); err != nil {
					return err
				}
				rt.Msg(cmd.ErrOrStderr(), "Removed the record of link %s", args[0])
				return nil
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := linkFallback(rt, args[0], ref)
			_, err = cli.OnNotFound(cmd.Context(), fb, func(ctx context.Context) (*link.PublicShare, error) {
				return nil, b.Links().Remove(ctx, ref)
			}, lookup, func(ctx context.Context, s *link.PublicShare) (*link.PublicShare, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return s, sa.RemovePublicLink(ctx, ref, false)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Removed link %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&dbOnly, "db-only", false, "remove the record only, without the checks of the gateway (for a link whose owner is gone)")
	cli.AdminFlag(cmd.Flags(), "db-only")
	return cmd
}

func completeShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
		shares, err := b.Shares().List(ctx, nil)
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
	return completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
		shares, err := b.Links().List(ctx, nil)
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

// An id can have a description after a tab.
func completeIDs(rt *cli.Runtime, list func(ctx context.Context, b backend.Backend) ([]string, error)) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if len(args) > 0 {
			return nil, none
		}
		// Completion does not run the PersistentPreRunE, so load the settings here.
		if err := rt.Load(cmd.Flags()); err != nil {
			return nil, none
		}
		b, err := rt.Backend(cmd.Context())
		if err != nil {
			return nil, none
		}
		ids, err := list(cmd.Context(), b)
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
	case "uploader":
		return permissions.NewUploaderRole().CS3ResourcePermissions(), nil
	case "manager":
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

var shareHeader = table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Type",
	"Grantee.Idp", "Grantee.OpaqueId", "Created", "Updated"}

func renderShares(w io.Writer, shares []*collaboration.Share) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(shareHeader)
	for _, s := range shares {
		t.AppendRow(shareRow(s))
	}
	t.Render()
}

func shareRow(s *collaboration.Share) table.Row {
	var idp, opaque string
	switch s.GetGrantee().GetType() {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		idp, opaque = s.Grantee.GetUserId().Idp, s.Grantee.GetUserId().OpaqueId
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		idp, opaque = s.Grantee.GetGroupId().Idp, s.Grantee.GetGroupId().OpaqueId
	}
	return table.Row{s.GetId().GetOpaqueId(), s.GetOwner().GetIdp(), s.GetOwner().GetOpaqueId(), s.GetResourceId().String(), s.GetPermissions().String(),
		s.GetGrantee().GetType().String(), idp, opaque, time.Unix(int64(s.GetCtime().GetSeconds()), 0), time.Unix(int64(s.GetMtime().GetSeconds()), 0)}
}

var linkHeader = table.Row{"#", "Owner.Idp", "Owner.OpaqueId", "ResourceId", "Permissions", "Token", "Expiration", "Created", "Updated", "Description"}

func renderPublicShares(w io.Writer, shares []*link.PublicShare) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(linkHeader)
	for _, s := range shares {
		t.AppendRow(linkRow(s))
	}
	t.Render()
}

func linkRow(s *link.PublicShare) table.Row {
	return table.Row{s.GetId().GetOpaqueId(), s.GetOwner().GetIdp(), s.GetOwner().GetOpaqueId(), s.GetResourceId().String(), s.GetPermissions().String(), s.GetToken(),
		s.GetExpiration().String(), time.Unix(int64(s.GetCtime().GetSeconds()), 0), time.Unix(int64(s.GetMtime().GetSeconds()), 0), s.GetDescription()}
}
