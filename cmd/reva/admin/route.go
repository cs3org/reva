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

package admin

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

// RouteCommand returns `route`, the operator command for the routing table
// of the storage registry. It is in the storage group, not the fleet group,
// so it is not in Commands.
func RouteCommand(r *cli.Runtime) *cobra.Command {
	storage := func(ctx context.Context) (adminpb.StorageAdminClient, context.Context, error) {
		c, ctx, err := r.Admin(ctx)
		if err != nil {
			return nil, nil, err
		}
		return adminpb.NewStorageAdminClient(c.Conn), ctx, nil
	}
	routes := func(ctx context.Context) ([]*adminpb.Route, error) {
		c, ctx, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		res, err := c.ListRoutes(ctx, &adminpb.ListRoutesRequest{})
		return res.GetRoutes(), adminErr(err)
	}
	completeRoutes := cli.Complete(r, 1, func(ctx context.Context, _ string) ([]string, error) {
		rs, err := routes(ctx)
		var paths []string
		for _, rt := range rs {
			paths = append(paths, rt.Path+"\t"+rt.MountId)
		}
		return paths, err
	})

	cmd := &cobra.Command{
		Use:   "route",
		Short: "Show and change the routing table of the storage registry",
		Long: "Show and change the routing table of the storage registry: the mount that\n" +
			"serves each path. A change is written once, then every storage registry\n" +
			"instance reloads the table. An instance that did not reload is reported,\n" +
			"and the command fails, because it serves the old table.",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the routes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rs, err := routes(cmd.Context())
			if err != nil {
				return err
			}
			t := cli.Table{Headers: []string{"PATH", "MOUNT ID"}, Items: rs}
			for _, rt := range rs {
				t.Rows = append(t.Rows, []string{rt.Path, rt.MountId})
			}
			return r.Print(cmd.OutOrStdout(), t, nil)
		},
	}
	resolve := &cobra.Command{
		Use:               "resolve <path>",
		Short:             "Show the mounts that serve a path, and their storage provider instances",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRoutes,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, ctx, err := storage(cmd.Context())
			if err != nil {
				return err
			}
			res, err := c.ResolvePath(ctx, &adminpb.ResolvePathRequest{Path: args[0]})
			if err != nil {
				return adminErr(err)
			}
			t := cli.Table{Headers: []string{"MOUNT ID", "MOUNT PATH", "ADDRESS", "PROVIDERS"}, Items: res}
			for _, m := range res.Mounts {
				t.Rows = append(t.Rows, []string{m.MountId, m.MountPath, m.Address, strings.Join(m.ProviderNodes, ",")})
			}
			return r.Print(cmd.OutOrStdout(), t, nil)
		},
	}
	add := &cobra.Command{
		Use:     "add <path> <mount-id>",
		Short:   "Route a path to a mount, or change the mount of a routed path",
		Example: "  reva route add /eos/user/z eoshome-i05",
		Args:    cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 1 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			// The routing table has no list of mounts, so offer the mounts
			// that it uses already.
			return cli.Complete(r, -1, func(ctx context.Context, _ string) ([]string, error) {
				rs, err := routes(ctx)
				var ids []string
				for _, rt := range rs {
					ids = append(ids, rt.MountId)
				}
				slices.Sort(ids)
				return slices.Compact(ids), err
			})(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := r.Confirm(fmt.Sprintf("route %s to %s", args[0], args[1])); err != nil {
				return err
			}
			c, ctx, err := storage(cmd.Context())
			if err != nil {
				return err
			}
			res, err := c.AddRoute(ctx, &adminpb.AddRouteRequest{Path: args[0], MountId: args[1]})
			if err != nil {
				return adminErr(err)
			}
			if err := r.Print(cmd.OutOrStdout(), res, func(w io.Writer) {
				fmt.Fprintf(w, "%s is routed to %s\n", res.Route.GetPath(), res.Route.GetMountId())
			}); err != nil {
				return err
			}
			return staleErr(cmd.ErrOrStderr(), res.Stale)
		},
	}
	remove := &cobra.Command{
		Use:               "remove <path>",
		Short:             "Remove the route of a path",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRoutes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := r.Confirm("remove the route of " + args[0]); err != nil {
				return err
			}
			c, ctx, err := storage(cmd.Context())
			if err != nil {
				return err
			}
			res, err := c.RemoveRoute(ctx, &adminpb.RemoveRouteRequest{Path: args[0]})
			if err != nil {
				return adminErr(err)
			}
			if err := r.Print(cmd.OutOrStdout(), res, func(w io.Writer) {
				fmt.Fprintf(w, "the route of %s is removed\n", res.Path)
			}); err != nil {
				return err
			}
			return staleErr(cmd.ErrOrStderr(), res.Stale)
		},
	}
	cmd.AddCommand(list, resolve, add, remove)
	return cli.AdminCommand(cmd)
}

// staleErr reports the storage registry instances that did not reload the
// routing table. The change is written, but these instances serve the old
// table until they reload, so the command must fail.
func staleErr(w io.Writer, stale []*adminpb.StaleInstance) error {
	if len(stale) == 0 {
		return nil
	}
	for _, s := range stale {
		fmt.Fprintf(w, "stale: %s: %s\n", s.Node, s.Error)
	}
	return fmt.Errorf("the change is written, but %d storage registry instances did not reload it: run `reva invoke <instance> route_reload` for each one", len(stale))
}
