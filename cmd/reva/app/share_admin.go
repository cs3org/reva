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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// shareListOptions are the operator options of `share list` and `link list`.
type shareListOptions struct {
	owner, space, path    string
	all, orphans, expired bool
}

func (o *shareListOptions) addFlags(cmd *cobra.Command, noun string, withPath bool) {
	f := cmd.Flags()
	f.StringVar(&o.owner, "owner", "", "the "+noun+" of this owner")
	f.BoolVar(&o.all, "all", false, "the "+noun+" of every user, not only yours (give a filter too)")
	f.StringVar(&o.space, "space", "", "the "+noun+" in this space: a space id (storage$space), a project name, or a path")
	f.BoolVar(&o.orphans, "orphans", false, "only the orphaned "+noun+", whose resource is gone")
	f.BoolVar(&o.expired, "expired", false, "only the expired "+noun)
	names := []string{"owner", "all", "space", "orphans", "expired"}
	if withPath {
		f.StringVar(&o.path, "path", "", "the "+noun+" of this path, resolved as --owner (same as PATH with an operator option)")
		names = append(names, "path")
	}
	for _, n := range names {
		cli.AdminFlag(f, n)
	}
}

func adminListShares(cmd *cobra.Command, rt *cli.Runtime, args []string, resID string, o shareListOptions) error {
	p := o.path
	switch {
	case len(args) == 1 && p != "":
		return errors.New("pass either PATH or --path, not both")
	case len(args) == 1:
		p = args[0]
	}
	switch {
	case p != "" && resID != "":
		return errors.New("pass either a path or --by-resource-id, not both")
	case o.owner == "" && o.space == "" && p == "" && resID == "":
		return errors.New("pass --owner, --space, --by-resource-id or a path: the share table is too large to list without a filter")
	case p != "" && o.owner == "":
		return errors.New("a path needs --owner: the path is resolved as that user")
	}
	owner, err := resolveUser(cmd.Context(), rt, o.owner)
	if err != nil {
		return err
	}
	var filters []*collaboration.Filter
	if owner != nil {
		filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_OWNER, Term: &collaboration.Filter_Owner{Owner: owner}})
	}
	if resID != "" {
		id, err := parseResourceID(resID)
		if err != nil {
			return err
		}
		filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_RESOURCE_ID, Term: &collaboration.Filter_ResourceId{ResourceId: id}})
	}
	sa, ctx, err := sharesAdmin(cmd.Context(), rt)
	if err != nil {
		return err
	}
	stream, err := sa.ListShares(ctx, &adminpb.ListSharesRequest{
		Filters: filters, Space: adminSpaceRef(o.space), Path: p,
		Orphans: only(o.orphans), Expired: only(o.expired), ResolvePaths: true,
	})
	if err != nil {
		return err
	}
	shares, err := recvAll(stream)
	if err != nil {
		return err
	}
	return rt.Print(cmd.OutOrStdout(), shares, func(w io.Writer) { renderAdminShares(w, shares) })
}

func adminListLinks(cmd *cobra.Command, rt *cli.Runtime, args []string, resID string, o shareListOptions) error {
	switch {
	case len(args) == 1:
		return errors.New("the admin plane cannot list the links of a path: use --owner, --space or --by-resource-id")
	case o.owner == "" && o.space == "" && resID == "":
		return errors.New("pass --owner, --space or --by-resource-id: the link table is too large to list without a filter")
	}
	owner, err := resolveUser(cmd.Context(), rt, o.owner)
	if err != nil {
		return err
	}
	var filters []*link.ListPublicSharesRequest_Filter
	if owner != nil {
		filters = append(filters, &link.ListPublicSharesRequest_Filter{
			Type: link.ListPublicSharesRequest_Filter_TYPE_OWNER, Term: &link.ListPublicSharesRequest_Filter_Owner{Owner: owner},
		})
	}
	if resID != "" {
		id, err := parseResourceID(resID)
		if err != nil {
			return err
		}
		filters = append(filters, &link.ListPublicSharesRequest_Filter{
			Type: link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID, Term: &link.ListPublicSharesRequest_Filter_ResourceId{ResourceId: id},
		})
	}
	sa, ctx, err := sharesAdmin(cmd.Context(), rt)
	if err != nil {
		return err
	}
	stream, err := sa.ListPublicLinks(ctx, &adminpb.ListPublicLinksRequest{
		Filters: filters, Space: adminSpaceRef(o.space),
		Orphans: only(o.orphans), Expired: only(o.expired), ResolvePaths: true,
	})
	if err != nil {
		return err
	}
	links, err := recvAll(stream)
	if err != nil {
		return err
	}
	return rt.Print(cmd.OutOrStdout(), links, func(w io.Writer) { renderAdminLinks(w, links) })
}

func shareSyncExpiryCommand(rt *cli.Runtime) *cobra.Command {
	return cli.AdminCommand(&cobra.Command{
		Use:               "sync-expiry SHARE_ID",
		Short:             "Remove the storage grant of an expired share now, and keep the share",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return shareOperation(cmd, rt, fmt.Sprintf("remove the storage grant of share %s", args[0]),
				func(ctx context.Context, sa adminpb.SharesAdminClient) (*adminpb.AdminShare, error) {
					return sa.SyncShareExpiration(ctx, &adminpb.SyncShareExpirationRequest{Ref: shareRef(args[0])})
				})
		},
	})
}

func shareTransferCommand(rt *cli.Runtime) *cobra.Command {
	return cli.AdminCommand(&cobra.Command{
		Use:               "transfer SHARE_ID USER",
		Short:             "Give a share to a new initiator, who must be allowed to share the resource",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			initiator, err := resolveUser(cmd.Context(), rt, args[1])
			if err != nil {
				return err
			}
			return shareOperation(cmd, rt, fmt.Sprintf("give share %s to %s", args[0], args[1]),
				func(ctx context.Context, sa adminpb.SharesAdminClient) (*adminpb.AdminShare, error) {
					return sa.TransferShare(ctx, &adminpb.TransferShareRequest{Ref: shareRef(args[0]), Initiator: initiator})
				})
		},
	})
}

func shareMarkOrphanCommand(rt *cli.Runtime) *cobra.Command {
	return cli.AdminCommand(&cobra.Command{
		Use:               "mark-orphan SHARE_ID",
		Short:             "Mark a share orphaned, which hides it from users",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return shareOperation(cmd, rt, fmt.Sprintf("mark share %s orphaned", args[0]),
				func(ctx context.Context, sa adminpb.SharesAdminClient) (*adminpb.AdminShare, error) {
					return sa.MarkShareOrphan(ctx, &adminpb.MarkShareOrphanRequest{Ref: shareRef(args[0])})
				})
		},
	})
}

func linkTransferCommand(rt *cli.Runtime) *cobra.Command {
	return cli.AdminCommand(&cobra.Command{
		Use:               "transfer LINK_ID|TOKEN USER",
		Short:             "Give a public link to a new initiator, who must be allowed to share the resource",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			initiator, err := resolveUser(cmd.Context(), rt, args[1])
			if err != nil {
				return err
			}
			return linkOperation(cmd, rt, fmt.Sprintf("give link %s to %s", args[0], args[1]),
				func(ctx context.Context, sa adminpb.SharesAdminClient) (*adminpb.AdminPublicShare, error) {
					return sa.TransferPublicLink(ctx, &adminpb.TransferPublicLinkRequest{Ref: adminLinkRef(args[0]), Initiator: initiator})
				})
		},
	})
}

func linkMarkOrphanCommand(rt *cli.Runtime) *cobra.Command {
	return cli.AdminCommand(&cobra.Command{
		Use:               "mark-orphan LINK_ID|TOKEN",
		Short:             "Mark a public link orphaned, which hides it from users",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return linkOperation(cmd, rt, fmt.Sprintf("mark link %s orphaned", args[0]),
				func(ctx context.Context, sa adminpb.SharesAdminClient) (*adminpb.AdminPublicShare, error) {
					return sa.MarkPublicLinkOrphan(ctx, &adminpb.MarkPublicLinkOrphanRequest{Ref: adminLinkRef(args[0])})
				})
		},
	})
}

// shareOperation runs an operator verb on one share, after a confirmation,
// and prints the share.
func shareOperation(cmd *cobra.Command, rt *cli.Runtime, prompt string, call func(context.Context, adminpb.SharesAdminClient) (*adminpb.AdminShare, error)) error {
	s, err := operation(cmd, rt, prompt, call)
	if err != nil {
		return err
	}
	return rt.Print(cmd.OutOrStdout(), s, func(w io.Writer) { renderAdminShares(w, []*adminpb.AdminShare{s}) })
}

func linkOperation(cmd *cobra.Command, rt *cli.Runtime, prompt string, call func(context.Context, adminpb.SharesAdminClient) (*adminpb.AdminPublicShare, error)) error {
	l, err := operation(cmd, rt, prompt, call)
	if err != nil {
		return err
	}
	return rt.Print(cmd.OutOrStdout(), l, func(w io.Writer) { renderAdminLinks(w, []*adminpb.AdminPublicShare{l}) })
}

func operation[M proto.Message](cmd *cobra.Command, rt *cli.Runtime, prompt string, call func(context.Context, adminpb.SharesAdminClient) (M, error)) (M, error) {
	var zero M
	if err := rt.Confirm(prompt); err != nil {
		return zero, err
	}
	sa, ctx, err := sharesAdmin(cmd.Context(), rt)
	if err != nil {
		return zero, err
	}
	return call(ctx, sa)
}

func shareMoveCommand(rt *cli.Runtime) *cobra.Command {
	var owner, space, resolveAs string
	var applyGrant, dryRun bool
	cmd := &cobra.Command{
		Use:   "move FROM_PATH TO_PATH",
		Short: "Point the shares and the links under a path at the new path, after a data move",
		Long: "Point the shares and the public links under FROM_PATH at the same resources\n" +
			"under TO_PATH, after the data was moved. A share is under FROM_PATH by its\n" +
			"current path or, if its resource is gone, by its path at creation. Orphans\n" +
			"are not moved. The command shows what it will move, and asks first.",
		Example:           "  reva share move /eos/user/a/alice/old /eos/project/n/new --owner alice --dry-run",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if owner == "" && space == "" {
				return errors.New("pass --owner, --space or both: the share tables are too large to read without a filter")
			}
			ownerID, err := resolveUser(cmd.Context(), rt, owner)
			if err != nil {
				return err
			}
			asID, err := resolveUser(cmd.Context(), rt, resolveAs)
			if err != nil {
				return err
			}
			sa, ctx, err := sharesAdmin(cmd.Context(), rt)
			if err != nil {
				return err
			}
			req := &adminpb.MoveSharesRequest{
				FromPath: args[0], ToPath: args[1], Owner: ownerID, Space: adminSpaceRef(space),
				As: asID, ApplyGrant: applyGrant, DryRun: true,
			}
			move := func() ([]*adminpb.MovedShare, error) {
				stream, err := sa.MoveShares(ctx, req)
				if err != nil {
					return nil, err
				}
				return recvAll(stream)
			}
			plan, err := move()
			if err != nil {
				return err
			}
			if dryRun || len(plan) == 0 {
				if len(plan) == 0 {
					rt.Msg(cmd.ErrOrStderr(), "Nothing to move under %s", args[0])
				}
				return rt.Print(cmd.OutOrStdout(), plan, func(w io.Writer) { renderMoved(w, plan) })
			}
			if rt.Output() != "json" {
				renderMoved(cmd.ErrOrStderr(), plan)
			}
			if err := rt.Confirm(fmt.Sprintf("move %d shares and links to %s", len(plan), args[1])); err != nil {
				return err
			}
			req.DryRun = false
			moved, err := move()
			if perr := rt.Print(cmd.OutOrStdout(), moved, func(w io.Writer) { renderMoved(w, moved) }); perr != nil {
				return perr
			}
			return err
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "move the shares and links of this owner")
	cmd.Flags().StringVar(&space, "space", "", "move the shares and links of this space: a space id, a project name, or a path")
	cmd.Flags().StringVar(&resolveAs, "resolve-as", "", "the user to resolve the paths as (default: the owner, then the initiator of each share)")
	cmd.Flags().BoolVar(&applyGrant, "apply-grant", false, "also add the grant of each share on its new resource")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only show what would be moved")
	return cli.AdminCommand(cmd)
}

// sharesAdmin returns a client of the SharesAdmin service and ctx for an
// admin-plane call.
func sharesAdmin(ctx context.Context, rt *cli.Runtime) (adminpb.SharesAdminClient, context.Context, error) {
	c, ctx, err := rt.Admin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return adminpb.NewSharesAdminClient(c.Conn), ctx, nil
}

func recvAll[M any](stream grpc.ServerStreamingClient[M]) ([]*M, error) {
	var out []*M
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
}

// adminSpaceRef reads --space: a path, a space id (storage$space) or a
// project name.
func adminSpaceRef(v string) *adminpb.SpaceRef {
	switch {
	case v == "":
		return nil
	case strings.HasPrefix(v, "/"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: v}}
	case strings.Contains(v, "$"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: &provider.StorageSpaceId{OpaqueId: v}}}
	}
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: v}}
}

// adminLinkRef reads a link argument of an operator verb. The sql manager,
// which the admin operations need, uses numbers as ids.
func adminLinkRef(arg string) *link.PublicShareReference {
	if strings.Trim(arg, "0123456789") == "" {
		return publicShareRef(arg)
	}
	return &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: arg}}
}

// resolveUser returns the CS3 id of the user that the operator names, or nil
// for no name. The admin services take the full id, with its idp and type,
// because a username does not name one account; the user plane resolves it
// over either transport.
func resolveUser(ctx context.Context, rt *cli.Runtime, name string) (*userpb.UserId, error) {
	if name == "" {
		return nil, nil
	}
	b, err := rt.Backend(ctx)
	if err != nil {
		return nil, err
	}
	u, err := b.Users().ByClaim(ctx, "username", name)
	switch {
	case cli.IsNotFound(err), err == nil && u.GetId().GetOpaqueId() == "":
		return nil, fmt.Errorf("no user %s", name)
	case err != nil:
		return nil, fmt.Errorf("looking up user %s: %w", name, err)
	}
	return u.GetId(), nil
}

func only(b bool) adminpb.ShareSelection {
	if b {
		return adminpb.ShareSelection_SHARE_SELECTION_ONLY
	}
	return adminpb.ShareSelection_SHARE_SELECTION_DEFAULT
}

// renderAdminShares writes the table of the user plane, with the ORPHAN and
// PATH columns when a share has them.
func renderAdminShares(w io.Writer, shares []*adminpb.AdminShare) {
	var orphan, path bool
	for _, s := range shares {
		orphan, path = orphan || s.GetOrphan(), path || s.GetPath() != ""
	}
	rows := make([]table.Row, 0, len(shares))
	for _, s := range shares {
		rows = append(rows, withAdminColumns(shareRow(s.GetShare()), orphan, path, yes(s.GetOrphan()), s.GetPath()))
	}
	renderRows(w, withAdminColumns(shareHeader, orphan, path, "Orphan", "Path"), rows)
}

func renderAdminLinks(w io.Writer, links []*adminpb.AdminPublicShare) {
	var orphan, path bool
	for _, l := range links {
		orphan, path = orphan || l.GetOrphan(), path || l.GetPath() != ""
	}
	rows := make([]table.Row, 0, len(links))
	for _, l := range links {
		rows = append(rows, withAdminColumns(linkRow(l.GetPublicShare()), orphan, path, yes(l.GetOrphan()), l.GetPath()))
	}
	renderRows(w, withAdminColumns(linkHeader, orphan, path, "Orphan", "Path"), rows)
}

// withAdminColumns adds to r the cells of the ORPHAN and PATH columns, when
// the table has them.
func withAdminColumns(r table.Row, orphan, path bool, orphanCell, pathCell string) table.Row {
	r = append(table.Row{}, r...)
	if orphan {
		r = append(r, orphanCell)
	}
	if path {
		r = append(r, pathCell)
	}
	return r
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func renderMoved(w io.Writer, items []*adminpb.MovedShare) {
	var failed bool
	for _, it := range items {
		failed = failed || it.GetError() != ""
	}
	header := table.Row{"Type", "#", "From", "To"}
	if failed {
		header = append(header, "Error")
	}
	rows := make([]table.Row, 0, len(items))
	for _, it := range items {
		r := table.Row{"share", it.GetShare().GetShare().GetId().GetOpaqueId(), it.GetFromPath(), it.GetToPath()}
		if it.GetPublicShare() != nil {
			r[0], r[1] = "link", it.GetPublicShare().GetPublicShare().GetId().GetOpaqueId()
		}
		if failed {
			r = append(r, it.GetError())
		}
		rows = append(rows, r)
	}
	renderRows(w, header, rows)
}

func renderRows(w io.Writer, header table.Row, rows []table.Row) {
	t := cli.NewTable(w)
	t.AppendHeader(header)
	t.AppendRows(rows)
	t.Render()
}

func isAlreadyExists(err error) bool {
	if se, ok := errors.AsType[*cli.StatusError](err); ok {
		return se.Code == rpc.Code_CODE_ALREADY_EXISTS
	}
	return status.Code(err) == codes.AlreadyExists
}

// existingShare returns the share of grantee on the resource id, or nil if
// the caller cannot see it.
func existingShare(ctx context.Context, b backend.Backend, id *provider.ResourceId, grantee *provider.Grantee) (*collaboration.Share, error) {
	shares, err := b.Shares().List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, s := range shares {
		if s.GetGrantee().GetType() == grantee.GetType() && granteeName(s.GetGrantee()) == granteeName(grantee) {
			return s, nil
		}
	}
	return nil, nil
}

func granteeName(g *provider.Grantee) string {
	return g.GetUserId().GetOpaqueId() + g.GetGroupId().GetOpaqueId()
}
