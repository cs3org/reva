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
	"maps"
	"slices"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/spf13/cobra"
)

const spaceArgHelp = "SPACE is a path in the space, its id or its name."

var spaceStatuses = []string{"creating", "active", "archiving", "archived"}

// spaceCommand is the space noun. `space quota` adds its subcommand here.
func spaceCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "space", Short: "Manage storage spaces"}
	cmd.AddCommand(
		spaceListCommand(rt), spaceInfoCommand(rt), spaceUpdateCommand(rt),
		cli.AdminCommand(spaceCheckCommand(rt)),
		cli.AdminCommand(spaceSetStatusCommand(rt)),
		cli.AdminCommand(spaceSetOwnerCommand(rt)),
		cli.AdminCommand(spaceRegisterCommand(rt)),
		cli.AdminCommand(spaceUnregisterCommand(rt)),
	)
	return cmd
}

// spaceRef is the SpacesAdmin reference of arg. A space id has a $
// between the storage id and the space id.
func spaceRef(arg string) *adminpb.SpaceRef {
	switch {
	case strings.HasPrefix(arg, "/"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: arg}}
	case strings.Contains(arg, "$"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: &provider.StorageSpaceId{OpaqueId: arg}}}
	}
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: arg}}
}

func spacesAdmin(ctx context.Context, rt *cli.Runtime) (adminpb.SpacesAdminClient, context.Context, error) {
	ctx, err := rt.AdminContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	sa, err := rt.SpacesAdmin(ctx)
	return sa, ctx, err
}

func recvAll[M any](recv func() (M, error)) ([]M, error) {
	var out []M
	for {
		m, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
}

// userSpace finds the space ref among the spaces of the caller.
func userSpace(ctx context.Context, b backend.Backend, ref string) (*provider.StorageSpace, error) {
	list, err := b.Spaces().List(ctx)
	if err != nil {
		return nil, err
	}
	return backend.FindSpace(list, ref)
}

// spaceFallback returns the owner fallback of the space commands on ref, and
// the lookup of the space with the Admin API.
func spaceFallback(rt *cli.Runtime, ref string) (cli.Fallback, func(context.Context) (*adminpb.AdminSpace, error)) {
	return cli.Fallback{Gate: rt, What: "space " + ref}, func(ctx context.Context) (*adminpb.AdminSpace, error) {
		sa, err := rt.SpacesAdmin(ctx)
		if err != nil {
			return nil, err
		}
		return sa.GetSpace(ctx, &adminpb.GetSpaceRequest{Ref: spaceRef(ref)})
	}
}

func spaceListCommand(rt *cli.Runtime) *cobra.Command {
	var typ string
	var all bool
	var req adminpb.ListSpacesRequest
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the spaces you can see, or with an operator option the spaces of every owner",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if rt.Plane() == cli.AdminPlane {
				sa, ctx, err := spacesAdmin(ctx, rt)
				if err != nil {
					return err
				}
				req.Type = typ
				st, err := sa.ListSpaces(ctx, &req)
				if err != nil {
					return err
				}
				list, err := recvAll(st.Recv)
				if err != nil {
					return err
				}
				return rt.Print(cmd.OutOrStdout(), list, func(w io.Writer) { renderSpaces(w, list) })
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			list, err := b.Spaces().List(ctx)
			if err != nil {
				return err
			}
			if typ != "" {
				list = slices.DeleteFunc(list, func(s *provider.StorageSpace) bool { return s.GetSpaceType() != typ })
			}
			return rt.Print(cmd.OutOrStdout(), list, func(w io.Writer) { renderSpaces(w, wrapSpaces(list)) })
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "only the spaces of this type: personal, project or explorer")
	cmd.Flags().BoolVar(&all, "all", false, "list the spaces of the catalogue, of every owner")
	cmd.Flags().StringVar(&req.Owner, "owner", "", "only the spaces of this owner")
	cmd.Flags().StringVar(&req.Member, "member", "", "only the spaces that this user owns or can access through a role")
	cmd.Flags().StringVar(&req.Status, "status", "", "only the spaces in this status: "+strings.Join(spaceStatuses, ", "))
	cmd.Flags().StringVar(&req.Profile, "profile", "", "only the spaces of this profile (not implemented yet)")
	cmd.Flags().BoolVar(&req.IncludeArchived, "include-archived", false, "include the archived spaces")
	for _, f := range []string{"all", "owner", "member", "status", "profile", "include-archived"} {
		cli.AdminFlag(cmd.Flags(), f)
	}
	_ = cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions([]string{"personal", "project", "explorer"}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("status", cobra.FixedCompletions(spaceStatuses, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func spaceInfoCommand(rt *cli.Runtime) *cobra.Command {
	var catalogue bool
	cmd := &cobra.Command{
		Use:               "info SPACE",
		Short:             "Show a space",
		Long:              "Show a space. " + spaceArgHelp + "\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSpaces(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if catalogue {
				sa, ctx, err := spacesAdmin(ctx, rt)
				if err != nil {
					return err
				}
				m, err := sa.GetSpace(ctx, &adminpb.GetSpaceRequest{Ref: spaceRef(args[0])})
				if err != nil {
					return err
				}
				return printSpace(cmd, rt, m)
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			fb, lookup := spaceFallback(rt, args[0])
			m, err := cli.OnNotFound(ctx, fb, func(ctx context.Context) (*adminpb.AdminSpace, error) {
				s, err := userSpace(ctx, b, args[0])
				if err != nil {
					return nil, err
				}
				return &adminpb.AdminSpace{Space: s}, nil
			}, lookup, nil)
			if err != nil {
				return err
			}
			return printSpace(cmd, rt, m)
		},
	}
	cmd.Flags().BoolVar(&catalogue, "catalogue", false, "show the record of the space catalogue, with its status and its columns")
	cli.AdminFlag(cmd.Flags(), "catalogue")
	return cmd
}

func spaceUpdateCommand(rt *cli.Runtime) *cobra.Command {
	var description, readme, thumbnail string
	cmd := &cobra.Command{
		Use:               "update SPACE",
		Short:             "Change the description, the readme or the thumbnail of a space",
		Long:              "Change the description, the readme or the thumbnail of a space. " + spaceArgHelp + "\nAn empty value clears the field.\n\n" + cli.FallbackHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSpaces(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := cmd.Flags().Changed
			// One UpdateStorageSpace call changes one field.
			var fields []*provider.UpdateStorageSpaceRequest_UpdateField
			if changed("description") {
				fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Description{Description: description}})
			}
			if changed("readme") {
				fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{Metadata: &provider.SpaceMetadata{Type: provider.SpaceMetadata_TYPE_README, Id: readme}}})
			}
			if changed("thumbnail") {
				fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{Metadata: &provider.SpaceMetadata{Type: provider.SpaceMetadata_TYPE_THUMBNAIL, Id: thumbnail}}})
			}
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			fb, lookup := spaceFallback(rt, args[0])
			m, err := cli.OnNotFound(ctx, fb, func(ctx context.Context) (*adminpb.AdminSpace, error) {
				s, err := userSpace(ctx, b, args[0])
				if err != nil {
					return nil, err
				}
				for _, f := range fields {
					u, err := b.Spaces().Update(ctx, &provider.UpdateStorageSpaceRequest{StorageSpace: s, Field: f})
					if err != nil {
						return nil, err
					}
					s = cmp.Or(u, s)
				}
				return &adminpb.AdminSpace{Space: s}, nil
			}, lookup, func(ctx context.Context, found *adminpb.AdminSpace) (*adminpb.AdminSpace, error) {
				sa, err := rt.SpacesAdmin(ctx)
				if err != nil {
					return nil, err
				}
				req := &adminpb.UpdateSpaceRequest{
					Ref:      &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: found.GetSpace().GetId()}},
					Fallback: cli.InFallback(ctx),
				}
				if changed("description") {
					req.Description = &description
				}
				if changed("readme") {
					req.Readme = &readme
				}
				if changed("thumbnail") {
					req.Thumbnail = &thumbnail
				}
				return sa.UpdateSpace(ctx, req)
			})
			if err != nil {
				return err
			}
			return printSpace(cmd, rt, m)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "the new description")
	cmd.Flags().StringVar(&readme, "readme", "", "the path of the new readme")
	cmd.Flags().StringVar(&thumbnail, "thumbnail", "", "the path of the new thumbnail")
	cmd.MarkFlagsOneRequired("description", "readme", "thumbnail")
	return cmd
}

func spaceCheckCommand(rt *cli.Runtime) *cobra.Command {
	var req adminpb.CheckSpacesRequest
	cmd := &cobra.Command{
		Use:   "check",
		Short: "List the spaces of the catalogue that have a problem",
		Long: "List the spaces of the catalogue that have a problem: an owner that is missing,\n" +
			"expiring or blocked, a path that does not stat as the owner, or a role group\n" +
			"that is not set, does not exist or has no members.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sa, ctx, err := spacesAdmin(cmd.Context(), rt)
			if err != nil {
				return err
			}
			st, err := sa.CheckSpaces(ctx, &req)
			if err != nil {
				return err
			}
			checks, err := recvAll(st.Recv)
			if err != nil {
				return err
			}
			t := cli.Table{Headers: []string{"Name", "Path", "Finding", "Detail"}, Items: checks}
			for _, c := range checks {
				for _, f := range c.Findings {
					t.Rows = append(t.Rows, []string{c.GetSpace().GetSpace().GetName(), c.GetSpace().GetSpace().GetRootInfo().GetPath(), f.Kind, f.Detail})
				}
			}
			return rt.Print(cmd.OutOrStdout(), t, nil)
		},
	}
	cmd.Flags().StringVar(&req.Status, "status", "", "only the spaces in this status: "+strings.Join(spaceStatuses, ", "))
	cmd.Flags().StringVar(&req.Owner, "owner", "", "only the spaces of this owner")
	_ = cmd.RegisterFlagCompletionFunc("status", cobra.FixedCompletions(spaceStatuses, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

// spaceChange runs a change of the catalogue after a confirmation, and
// prints the space.
func spaceChange(cmd *cobra.Command, rt *cli.Runtime, prompt string, call func(context.Context, adminpb.SpacesAdminClient) (*adminpb.AdminSpace, error)) error {
	if err := rt.Confirm(prompt); err != nil {
		return err
	}
	sa, ctx, err := spacesAdmin(cmd.Context(), rt)
	if err != nil {
		return err
	}
	m, err := call(ctx, sa)
	if err != nil {
		return err
	}
	return printSpace(cmd, rt, m)
}

func spaceSetStatusCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "set-status SPACE STATUS",
		Short: "Set the status of a space in the catalogue",
		Long:  "Set the status of a space in the catalogue: " + strings.Join(spaceStatuses, ", ") + ".\n" + spaceArgHelp,
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return spaceStatuses, cobra.ShellCompDirectiveNoFileComp
			}
			return completeSpaces(rt)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !slices.Contains(spaceStatuses, args[1]) {
				return fmt.Errorf("unknown status %q: use %s", args[1], strings.Join(spaceStatuses, ", "))
			}
			return spaceChange(cmd, rt, fmt.Sprintf("set the status of space %s to %s", args[0], args[1]),
				func(ctx context.Context, sa adminpb.SpacesAdminClient) (*adminpb.AdminSpace, error) {
					return sa.SetSpaceStatus(ctx, &adminpb.SetSpaceStatusRequest{Ref: spaceRef(args[0]), Status: args[1]})
				})
		},
	}
}

func spaceSetOwnerCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "set-owner SPACE USER",
		Short:             "Change the owner of a space in the catalogue",
		Long:              "Change the owner of a space in the catalogue. " + spaceArgHelp,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeSpaces(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			return spaceChange(cmd, rt, fmt.Sprintf("make %s the owner of space %s", args[1], args[0]),
				func(ctx context.Context, sa adminpb.SpacesAdminClient) (*adminpb.AdminSpace, error) {
					return sa.SetSpaceOwner(ctx, &adminpb.SetSpaceOwnerRequest{Ref: spaceRef(args[0]), Owner: args[1]})
				})
		},
	}
}

func spaceRegisterCommand(rt *cli.Runtime) *cobra.Command {
	var req adminpb.RegisterSpaceRequest
	cmd := &cobra.Command{
		Use:   "register NAME --owner USER",
		Short: "Add a space that the storage already serves to the catalogue",
		Long: "Add a space that the storage already serves to the catalogue. It does not\n" +
			"create the space on the storage. A value that is not given comes from the\n" +
			"configuration of the catalogue, or is required.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			return spaceChange(cmd, rt, fmt.Sprintf("register space %s, owned by %s, in the catalogue", req.Name, req.Owner),
				func(ctx context.Context, sa adminpb.SpacesAdminClient) (*adminpb.AdminSpace, error) {
					return sa.RegisterSpace(ctx, &req)
				})
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.Owner, "owner", "", "the owner")
	f.StringVar(&req.Path, "path", "", "the path of the space")
	f.StringVar(&req.StorageId, "storage-id", "", "the storage that serves the space")
	f.StringVar(&req.SpaceId, "space-id", "", "the space id (default: from the path)")
	f.StringVar(&req.Readers, "readers", "", "the group of the readers")
	f.StringVar(&req.Writers, "writers", "", "the group of the writers")
	f.StringVar(&req.Admins, "admins", "", "the group of the admins")
	f.StringVar(&req.Description, "description", "", "the description that users see")
	f.StringVar(&req.Status, "status", "", "the status (default active)")
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.RegisterFlagCompletionFunc("status", cobra.FixedCompletions(spaceStatuses, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func spaceUnregisterCommand(rt *cli.Runtime) *cobra.Command {
	var hard bool
	cmd := &cobra.Command{
		Use:   "unregister SPACE",
		Short: "Remove a space from the catalogue",
		Long: "Remove a space from the catalogue. It does not touch the data. " + spaceArgHelp + "\n" +
			"Without --hard, the record keeps its name and its space id taken.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSpaces(rt),
		RunE: func(cmd *cobra.Command, args []string) error {
			how := "soft-delete"
			if hard {
				how = "remove the record of"
			}
			return spaceChange(cmd, rt, fmt.Sprintf("%s space %s in the catalogue", how, args[0]),
				func(ctx context.Context, sa adminpb.SpacesAdminClient) (*adminpb.AdminSpace, error) {
					return sa.UnregisterSpace(ctx, &adminpb.UnregisterSpaceRequest{Ref: spaceRef(args[0]), Hard: hard})
				})
		},
	}
	cmd.Flags().BoolVar(&hard, "hard", false, "remove the record")
	return cmd
}

// completeSpaces completes the first argument with the names and the paths
// of the spaces of the caller.
func completeSpaces(rt *cli.Runtime) cobra.CompletionFunc {
	return completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
		list, err := b.Spaces().List(ctx)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, s := range list {
			if s.GetName() != "" {
				out = append(out, s.GetName()+"\t"+s.GetSpaceType())
			}
			if p := s.GetRootInfo().GetPath(); p != "" {
				out = append(out, p+"\t"+s.GetName())
			}
		}
		return out, nil
	})
}

func wrapSpaces(list []*provider.StorageSpace) []*adminpb.AdminSpace {
	out := make([]*adminpb.AdminSpace, len(list))
	for i, s := range list {
		out[i] = &adminpb.AdminSpace{Space: s}
	}
	return out
}

func spaceOwner(s *provider.StorageSpace) string {
	return cmp.Or(s.GetOwner().GetUsername(), s.GetOwner().GetId().GetOpaqueId())
}

// renderSpaces writes the same columns on both planes. Status and profile
// appear only when a space has them.
func renderSpaces(w io.Writer, list []*adminpb.AdminSpace) {
	withStatus := slices.ContainsFunc(list, func(m *adminpb.AdminSpace) bool { return m.Status != "" })
	withProfile := slices.ContainsFunc(list, func(m *adminpb.AdminSpace) bool { return m.Profile != "" })
	t := cli.Table{Headers: []string{"Name", "Type", "Owner", "Path"}}
	if withStatus {
		t.Headers = append(t.Headers, "Status")
	}
	if withProfile {
		t.Headers = append(t.Headers, "Profile")
	}
	t.Headers = append(t.Headers, "Id")
	for _, m := range list {
		s := m.GetSpace()
		r := []string{s.GetName(), s.GetSpaceType(), spaceOwner(s), s.GetRootInfo().GetPath()}
		if withStatus {
			r = append(r, m.Status)
		}
		if withProfile {
			r = append(r, m.Profile)
		}
		t.Rows = append(t.Rows, append(r, s.GetId().GetOpaqueId()))
	}
	t.Write(w)
}

// printSpace prints m. With -o json, a space of the user plane is the CS3
// message, and a space of the catalogue, which has a status, is AdminSpace.
func printSpace(cmd *cobra.Command, rt *cli.Runtime, m *adminpb.AdminSpace) error {
	var v any = m
	if m.Status == "" {
		v = m.GetSpace()
	}
	s := m.GetSpace()
	t := cli.Table{Headers: []string{"Field", "Value"}, Rows: [][]string{
		{"Name", s.GetName()},
		{"Id", s.GetId().GetOpaqueId()},
		{"Type", s.GetSpaceType()},
		{"Owner", spaceOwner(s)},
		{"Path", s.GetRootInfo().GetPath()},
		{"Description", s.GetDescription()},
		{"Readme", s.GetReadmeId()},
		{"Thumbnail", s.GetThumbnailId()},
	}}
	for _, r := range s.GetRoles() {
		var who []string
		for _, g := range r.GetRecipients() {
			who = append(who, cmp.Or(g.GetGroupId().GetOpaqueId(), g.GetUserId().GetOpaqueId()))
		}
		t.Rows = append(t.Rows, []string{"Role " + r.GetRoleName(), strings.Join(who, ", ")})
	}
	if m.Status != "" {
		t.Rows = append(t.Rows, []string{"Status", m.Status})
	}
	if m.Profile != "" {
		t.Rows = append(t.Rows, []string{"Profile", m.Profile})
	}
	for _, k := range slices.Sorted(maps.Keys(m.Extra)) {
		t.Rows = append(t.Rows, []string{k, m.Extra[k]})
	}
	return rt.Print(cmd.OutOrStdout(), v, t.Write)
}
