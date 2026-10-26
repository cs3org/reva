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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func shareCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "share", Short: "Manage shares with users and groups"}
	received := &cobra.Command{Use: "received", Short: "Manage the shares you received"}
	received.AddCommand(shareReceivedListCommand(rt), shareReceivedUpdateCommand(rt))
	cmd.AddCommand(shareCreateCommand(rt), shareListCommand(rt), shareGetCommand(rt), shareUpdateCommand(rt), shareRemoveCommand(rt), received)
	return cmd
}

// shareFallback returns the owner fallback of the share commands on id, and
// the lookup of the share with the Admin API.
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
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := shareFallback(rt, args[0])
			s, err := cli.OnNotFound(ctx, fb, func(ctx context.Context) (*collaboration.Share, error) {
				res, err := client.GetShare(ctx, &collaboration.GetShareRequest{Ref: shareRef(args[0])})
				if err != nil {
					return nil, err
				}
				return res.Share, cli.Check(res.Status)
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
			shares := make([]*collaboration.Share, 0, len(grantees))
			for _, g := range grantees {
				shareRes, err := client.CreateShare(ctx, &collaboration.CreateShareRequest{
					ResourceInfo: res.Info,
					Grant:        &collaboration.ShareGrant{Permissions: &collaboration.SharePermissions{Permissions: perm}, Grantee: g},
				})
				if err != nil {
					return err
				}
				if shareRes.Status.Code != rpc.Code_CODE_OK {
					return formatError(shareRes.Status)
				}
				shares = append(shares, shareRes.Share)
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderShares(w, shares) })
		},
	}
	cmd.Flags().StringArrayVar(&users, "with", nil, "user name to share with (repeatable)")
	cmd.Flags().StringArrayVar(&groups, "with-group", nil, "group to share with (repeatable)")
	cmd.Flags().StringVar(&idp, "idp", "", "the idp of the grantees (default: the idp of the caller)")
	cmd.Flags().StringVar(&role, "role", "viewer", "viewer, editor, manager, or denied")
	cmd.Flags().StringVar(&userType, "user-type", "primary", "the account type of the users")
	return cmd
}

func shareListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	cmd := &cobra.Command{
		Use:               "list [PATH]",
		Short:             "List the shares on a path, or all the shares you manage",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			id, err := resourceFilter(ctx, client, args, resID)
			if err != nil {
				return err
			}
			req := &collaboration.ListSharesRequest{}
			if id != nil {
				req.Filters = []*collaboration.Filter{share.ResourceIDFilter(id)}
			}
			res, err := client.ListShares(ctx, req)
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Shares, func(w io.Writer) { renderShares(w, res.Shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the shares of this resource (storage_id:opaque_id)")
	return cmd
}

// resourceFilter returns the resource that a list command filters on: the
// PATH argument or --by-resource-id. nil means no filter.
func resourceFilter(ctx context.Context, client gateway.GatewayAPIClient, args []string, resID string) (*provider.ResourceId, error) {
	switch {
	case len(args) == 1 && resID != "":
		return nil, errors.New("pass either PATH or --by-resource-id, not both")
	case resID != "":
		return parseResourceID(resID)
	case len(args) == 1:
		res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: args[0]}})
		if err != nil {
			return nil, err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return nil, formatError(res.Status)
		}
		return res.Info.Id, nil
	}
	return nil, nil
}

func shareUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:               "update SHARE_ID",
		Short:             "Change the role of a share",
		Long:              "Change the role of a share.\n\n" + cli.FallbackHelp,
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
			req := &collaboration.UpdateShareRequest{
				Ref: shareRef(args[0]),
				Field: &collaboration.UpdateShareRequest_UpdateField{
					Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
						Permissions: &collaboration.SharePermissions{Permissions: perm},
					},
				},
			}
			fb, lookup := shareFallback(rt, args[0])
			_, err = cli.OnNotFound(ctx, fb, func(ctx context.Context) (*collaboration.Share, error) {
				res, err := client.UpdateShare(ctx, req)
				if err != nil {
					return nil, err
				}
				return res.Share, cli.Check(res.Status)
			}, lookup, func(ctx context.Context, _ *collaboration.Share) (*collaboration.Share, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return sa.UpdateShare(ctx, req)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "new role of the grantee: viewer or editor")
	return cmd
}

func shareRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove SHARE_ID",
		Short:             "Remove a share",
		Long:              "Remove a share.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			fb, lookup := shareFallback(rt, args[0])
			_, err = cli.OnNotFound(ctx, fb, func(ctx context.Context) (*collaboration.Share, error) {
				res, err := client.RemoveShare(ctx, &collaboration.RemoveShareRequest{Ref: shareRef(args[0])})
				if err != nil {
					return nil, err
				}
				return nil, cli.Check(res.Status)
			}, lookup, func(ctx context.Context, s *collaboration.Share) (*collaboration.Share, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return s, sa.RemoveShare(ctx, shareRef(args[0]))
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Removed share %s", args[0])
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
		Use:   "update SHARE_ID",
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
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&state, "state", "pending", "new state: pending, accepted or rejected")
	return cmd
}

func linkCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "link", Short: "Create and manage public links"}
	cmd.AddCommand(linkCreateCommand(rt), linkListCommand(rt), linkGetCommand(rt), linkUpdateCommand(rt), linkRemoveCommand(rt))
	return cmd
}

// linkFallback returns the owner fallback of the link commands on ref, and
// the lookup of the link with the Admin API.
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
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			refs := linkRefs(args[0])
			fb := cli.Fallback{Gate: rt, What: "link " + args[0]}
			s, err := cli.OnNotFound(ctx, fb, func(ctx context.Context) (*link.PublicShare, error) {
				return firstFound(refs, func(ref *link.PublicShareReference) (*link.PublicShare, error) {
					res, err := client.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: ref})
					if err != nil {
						return nil, err
					}
					return res.Share, cli.Check(res.Status)
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

// linkRefs returns the references that arg can be: a link id, or a link
// token. The sql manager uses numbers as ids, so a number is only an id.
func linkRefs(arg string) []*link.PublicShareReference {
	refs := []*link.PublicShareReference{publicShareRef(arg)}
	if strings.Trim(arg, "0123456789") != "" {
		refs = append(refs, &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: arg}})
	}
	return refs
}

// firstFound returns the answer of get for the first reference that is found.
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

func linkListCommand(rt *cli.Runtime) *cobra.Command {
	var resID string
	cmd := &cobra.Command{
		Use:               "list [PATH]",
		Short:             "List the public links on a path, or all the links you created",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			id, err := resourceFilter(ctx, client, args, resID)
			if err != nil {
				return err
			}
			shares, err := listPublicShares(ctx, client, id)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderPublicShares(w, shares) })
		},
	}
	cmd.Flags().StringVar(&resID, "by-resource-id", "", "only the links of this resource (storage_id:opaque_id)")
	return cmd
}

func linkUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var role, description string
	cmd := &cobra.Command{
		Use:               "update LINK_ID",
		Short:             "Change the role or the description of a public link",
		Long:              "Change the role or the description of a public link.\n\n" + cli.FallbackHelp,
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
			ref := publicShareRef(args[0])
			fb, lookup := linkFallback(rt, args[0], ref)
			_, err = cli.OnNotFound(ctx, fb, func(ctx context.Context) (*link.PublicShare, error) {
				var s *link.PublicShare
				for _, u := range updates {
					res, err := client.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: ref, Update: u})
					if err != nil {
						return nil, err
					}
					if err := cli.Check(res.Status); err != nil {
						return nil, err
					}
					s = res.Share
				}
				return s, nil
			}, lookup, func(ctx context.Context, s *link.PublicShare) (*link.PublicShare, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				for _, u := range updates {
					if s, err = sa.UpdatePublicLink(ctx, &link.UpdatePublicShareRequest{Ref: ref, Update: u}); err != nil {
						return nil, err
					}
				}
				return s, nil
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "viewer", "new role of the link: viewer or editor")
	cmd.Flags().StringVar(&description, "description", "", "new description of the link")
	return cmd
}

func linkRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "remove LINK_ID",
		Short:             "Remove a public link",
		Long:              "Remove a public link.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completePublicShareIDs(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			ref := publicShareRef(args[0])
			fb, lookup := linkFallback(rt, args[0], ref)
			_, err = cli.OnNotFound(ctx, fb, func(ctx context.Context) (*link.PublicShare, error) {
				res, err := client.RemovePublicShare(ctx, &link.RemovePublicShareRequest{Ref: ref})
				if err != nil {
					return nil, err
				}
				return nil, cli.Check(res.Status)
			}, lookup, func(ctx context.Context, s *link.PublicShare) (*link.PublicShare, error) {
				sa, err := rt.SharesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				return s, sa.RemovePublicLink(ctx, ref)
			})
			if err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Removed link %s", args[0])
			return nil
		},
	}
}

// listPublicShares lists the links that the caller created. The creator
// filter needs the user id, so it asks the gateway who the token belongs to.
func listPublicShares(ctx context.Context, client gateway.GatewayAPIClient, resID *provider.ResourceId) ([]*link.PublicShare, error) {
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
	if resID != nil {
		filters = append(filters, publicshare.ResourceIDFilter(resID))
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
		res, err := client.ListShares(ctx, &collaboration.ListSharesRequest{})
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(res.Shares))
		for _, s := range res.Shares {
			ids = append(ids, s.Id.OpaqueId+"\t"+s.Grantee.GetUserId().GetOpaqueId()+s.Grantee.GetGroupId().GetOpaqueId())
		}
		return ids, nil
	})
}

func completePublicShareIDs(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, client gateway.GatewayAPIClient) ([]string, error) {
		shares, err := listPublicShares(ctx, client, nil)
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
