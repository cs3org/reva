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
	"slices"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const spaceRefHelp = "A space is a path, a space id (storage$id) or the name of a project."

func spaceQuotaCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "quota", Short: "Show and set the quota of spaces"}
	spaces := func(ctx context.Context) (adminpb.SpacesAdminClient, context.Context, error) {
		c, ctx, err := rt.Admin(ctx)
		if err != nil {
			return nil, nil, err
		}
		return adminpb.NewSpacesAdminClient(c.Conn), ctx, nil
	}

	get := &cobra.Command{
		Use:   "get [<space>]",
		Short: "Show the quota of a space, by default your home space",
		Long: "Show the quota of a space, by default your home space. " + spaceRefHelp + "\n\n" +
			cli.FallbackHelp,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			arg := "/home"
			if len(args) == 1 {
				arg = args[0]
			}
			userCall := func(ctx context.Context) (*adminpb.SpaceQuota, error) {
				ref := &provider.Reference{Path: arg}
				if !strings.HasPrefix(arg, "/") {
					var err error
					if ref, err = b.Files().ResolveSpace(ctx, arg); err != nil {
						return nil, err
					}
				}
				q, err := b.Spaces().Quota(ctx, ref)
				if err != nil {
					return nil, err
				}
				return &adminpb.SpaceQuota{Root: arg, Quota: &provider.Quota{QuotaMaxBytes: q.Total, RemainingBytes: q.Total - min(q.Used, q.Total)}}, nil
			}
			// The lookup is the read itself: GetSpaceQuota also finds the
			// spaces that are not in the catalogue, such as homes.
			lookup := func(ctx context.Context) (*adminpb.SpaceQuota, error) {
				c, ctx, err := spaces(ctx)
				if err != nil {
					return nil, err
				}
				return c.GetSpaceQuota(ctx, &adminpb.GetSpaceQuotaRequest{Ref: spaceRefArg(arg)})
			}
			q, err := cli.OnNotFound(cmd.Context(), cli.Fallback{Gate: rt, What: "space " + arg}, userCall, lookup, nil)
			if err != nil {
				return err
			}
			t := quotaTable([]*adminpb.SpaceQuota{q})
			t.Items = q
			return rt.Print(cmd.OutOrStdout(), t, nil)
		},
	}

	set := &cobra.Command{
		Use:   "set <space> <size>",
		Short: "Set the quota of a space",
		Long: "Set the quota of a space. " + spaceRefHelp + "\n\n" +
			"The size is in logical bytes, the size of the data that users see, for\n" +
			"example 500G or 1T (powers of 1000), or 1TiB (powers of 1024). The storage\n" +
			"keeps more than one copy of each byte, so the command also shows the\n" +
			"physical bytes. The storage chooses how it keeps the quota, from the space.",
		Example: "  reva space quota set /eos/project/c/cernbox 2T",
		Args: cobra.MatchAll(cobra.ExactArgs(2), func(_ *cobra.Command, args []string) error {
			var s byteSize
			return s.Set(args[1])
		}),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var size byteSize
			_ = size.Set(args[1])
			c, ctx, err := spaces(cmd.Context())
			if err != nil {
				return err
			}
			ref := spaceRefArg(args[0])
			old, err := c.GetSpaceQuota(ctx, &adminpb.GetSpaceQuotaRequest{Ref: ref})
			if err != nil {
				return err
			}
			newBytes, oldBytes := uint64(size), old.GetQuota().GetQuotaMaxBytes()
			var newPhysical uint64
			// The storage reports the copies only as physical bytes, so the
			// new physical size comes from the current ratio.
			if oldBytes > 0 && old.PhysicalMaxBytes%oldBytes == 0 {
				newPhysical = newBytes * (old.PhysicalMaxBytes / oldBytes)
			}
			prompt := fmt.Sprintf("set the quota of %s from %s to %s", old.Root, sizes(oldBytes, old.PhysicalMaxBytes), sizes(newBytes, newPhysical))
			if err := rt.Confirm(prompt); err != nil {
				return err
			}
			q, err := c.SetSpaceQuota(ctx, &adminpb.SetSpaceQuotaRequest{
				Ref:        ref,
				Quota:      &provider.Quota{QuotaMaxBytes: newBytes},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"quota_max_bytes"}},
			})
			if err != nil {
				return err
			}
			t := quotaTable([]*adminpb.SpaceQuota{q})
			t.Items = q
			return rt.Print(cmd.OutOrStdout(), t, nil)
		},
	}

	var minSize byteSize
	list := &cobra.Command{
		Use:   "list",
		Short: "List the quotas of the spaces that the storage keeps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, ctx, err := spaces(cmd.Context())
			if err != nil {
				return err
			}
			stream, err := c.ListSpaceQuotas(ctx, &adminpb.ListSpaceQuotasRequest{MinBytes: uint64(minSize)})
			if err != nil {
				return err
			}
			var qs []*adminpb.SpaceQuota
			for {
				q, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				qs = append(qs, q)
			}
			slices.SortFunc(qs, func(a, b *adminpb.SpaceQuota) int { return strings.Compare(a.Root, b.Root) })
			t := quotaTable(qs)
			t.Items = qs
			return rt.Print(cmd.OutOrStdout(), t, nil)
		},
	}
	list.Flags().Var(&minSize, "min", "only the quotas of at least this logical size, for example 10G")

	cmd.AddCommand(get, cli.AdminCommand(set), cli.AdminCommand(list))
	return cmd
}

// spaceRefArg reads the space argument: a path, a space id, which has a $,
// or a project name.
func spaceRefArg(s string) *adminpb.SpaceRef {
	switch {
	case strings.HasPrefix(s, "/"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: s}}
	case strings.Contains(s, "$"):
		return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: &provider.StorageSpaceId{OpaqueId: s}}}
	}
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: s}}
}

// quotaTable shows what is used, max - remaining, because operators reason
// in usage. Without a limit, the usage is not known.
func quotaTable(qs []*adminpb.SpaceQuota) cli.Table {
	t := cli.Table{Headers: []string{"ROOT", "USED", "MAX", "PHYSICAL MAX", "FILES", "FORM", "MOUNT ID"}}
	size := func(b uint64) string {
		if b == 0 {
			return "-"
		}
		return humanize.Bytes(b)
	}
	for _, q := range qs {
		l := q.GetQuota()
		used := "-"
		if l.GetQuotaMaxBytes() > 0 {
			used = humanize.Bytes(l.GetQuotaMaxBytes() - min(l.GetRemainingBytes(), l.GetQuotaMaxBytes()))
		}
		files := "-"
		if l.GetQuotaMaxFiles() > 0 {
			files = strconv.FormatUint(l.GetQuotaMaxFiles()-min(l.GetRemainingFiles(), l.GetQuotaMaxFiles()), 10) + "/" + strconv.FormatUint(l.GetQuotaMaxFiles(), 10)
		}
		t.Rows = append(t.Rows, []string{q.Root, used, size(l.GetQuotaMaxBytes()), size(q.PhysicalMaxBytes), files, dash(q.Form), dash(q.MountId)})
	}
	return t
}

// sizes shows logical bytes, and physical bytes when they are known.
func sizes(logical, physical uint64) string {
	s := fmt.Sprintf("%s (%d bytes)", humanize.Bytes(logical), logical)
	if physical > 0 {
		s += fmt.Sprintf(", %s physical", humanize.Bytes(physical))
	}
	return s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// byteSize is a flag value in bytes that takes human sizes, such as 10G.
type byteSize uint64

func (b *byteSize) String() string { return strconv.FormatUint(uint64(*b), 10) }
func (b *byteSize) Type() string   { return "size" }

func (b *byteSize) Set(s string) error {
	n, err := humanize.ParseBytes(s)
	if err != nil {
		return fmt.Errorf("%q is not a size, such as 500G or 1T", s)
	}
	*b = byteSize(n)
	return nil
}
