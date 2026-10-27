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
	"path"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/spf13/cobra"
)

type accessGrant struct {
	Who       string `json:"who"`
	Kind      string `json:"kind"`
	Role      string `json:"role,omitempty"`
	GrantedOn string `json:"granted_on"`
	Inherited bool   `json:"inherited"`
	Expires   string `json:"expires,omitempty"`
	ID        string `json:"id"`
}

func shareAuditCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "audit PATH",
		Short: "Show everyone who can reach a file, and how",
		Long: "Show who can reach a path, also through a share of a folder above it: every\n" +
			"share and link from the root of the space down to the path, and the folder\n" +
			"that each one is on.",
		Example:           "  reva share audit home:Documents/2026/report.pdf",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			target, err := rt.RemotePath(ctx, args[0])
			if err != nil {
				return err
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			spaces, err := b.Spaces().List(ctx)
			if err != nil {
				return err
			}
			space, err := backend.FindSpace(spaces, target)
			if err != nil {
				return err
			}
			grants := grantsReaching(ctx, rt, cmd, b, space.GetRootInfo().GetPath(), path.Clean(target))
			rt.Msg(cmd.ErrOrStderr(), "Path:  %s", target)
			rt.Msg(cmd.ErrOrStderr(), "Space: %s (%s)", space.Name, space.SpaceType)
			t := cli.Table{Headers: []string{"WHO", "ROLE", "KIND", "GRANTED ON"}, Items: grants}
			people, inherited := map[string]int{}, 0
			for _, g := range grants {
				on := g.GrantedOn
				if g.Inherited {
					on += "  (inherited)"
					inherited++
				}
				if g.Kind != "link" {
					people[g.Who]++
				}
				t.Rows = append(t.Rows, []string{g.Who, cmp.Or(g.Role, "-"), g.Kind, on})
			}
			if err := rt.Print(cmd.OutOrStdout(), t, nil); err != nil {
				return err
			}
			if len(grants) == 0 {
				rt.Msg(cmd.ErrOrStderr(), "Nothing grants access to this path on its own.")
			} else {
				rt.Msg(cmd.ErrOrStderr(), "%d grants reach this path, %d of them inherited from folders above.", len(grants), inherited)
			}
			for who, n := range people {
				if n > 1 {
					rt.Warnf("%s has access at %d levels: the storage decides which role applies", who, n)
				}
			}
			return nil
		},
	}
}

// grantsReaching lists the shares and links on root and on each folder
// between root and target. A level that cannot be read is a warning: the
// rest of the answer is still useful.
func grantsReaching(ctx context.Context, rt *cli.Runtime, cmd *cobra.Command, b backend.Backend, root, target string) []accessGrant {
	levels := []string{target}
	if root != "" && (target == root || strings.HasPrefix(target, strings.TrimSuffix(root, "/")+"/")) {
		levels = []string{root}
		at := root
		for seg := range strings.SplitSeq(strings.TrimPrefix(strings.TrimPrefix(target, root), "/"), "/") {
			if seg != "" {
				at = path.Join(at, seg)
				levels = append(levels, at)
			}
		}
	}
	var out []accessGrant
	for _, p := range levels {
		info, err := b.Files().Stat(ctx, pathRef(p))
		if cli.IsNotFound(err) {
			continue
		}
		if err != nil {
			rt.Warnf("cannot read %s, so a grant on it is not listed: %v", p, err)
			continue
		}
		inherited := p != target
		shares, err := b.Shares().List(ctx, info.Id)
		if err != nil {
			rt.Warnf("cannot list the shares on %s: %v", p, err)
		}
		for _, s := range shares {
			g := accessGrant{Kind: "user", GrantedOn: p, Inherited: inherited, ID: s.GetId().GetOpaqueId(),
				Role: permissions.RoleFromResourcePermissions(s.GetPermissions().GetPermissions()).Name, Expires: day(s.GetExpiration().GetSeconds())}
			if s.GetGrantee().GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP {
				g.Kind, g.Who = "group", s.GetGrantee().GetGroupId().GetOpaqueId()
			} else {
				g.Who = s.GetGrantee().GetUserId().GetOpaqueId()
			}
			out = append(out, g)
		}
		links, err := b.Links().List(ctx, info.Id)
		if err != nil {
			rt.Warnf("cannot list the links on %s: %v", p, err)
		}
		for _, l := range links {
			out = append(out, accessGrant{Kind: "link", Who: cmp.Or(l.GetDisplayName(), l.GetToken(), "(public link)"), GrantedOn: p,
				Inherited: inherited, ID: l.GetId().GetOpaqueId(), Expires: day(l.GetExpiration().GetSeconds()),
				Role: permissions.RoleFromResourcePermissions(l.GetPermissions().GetPermissions()).Name})
		}
	}
	return out
}

func day(s uint64) string {
	if s == 0 {
		return ""
	}
	return time.Unix(int64(s), 0).UTC().Format(time.DateOnly)
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

type doctorReport struct {
	Checks   []doctorCheck `json:"checks"`
	Problems int           `json:"problems"`
	Warnings int           `json:"warnings"`
}

func (r *doctorReport) add(name, status, detail, remedy string) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Status: status, Detail: detail, Remedy: remedy})
	switch status {
	case "problem":
		r.Problems++
	case "warning":
		r.Warnings++
	}
}

func doctorCommand(rt *cli.Runtime) *cobra.Command {
	var skipWrite bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that everything the client depends on works",
		Long: "Try what can go wrong, and say what to do about it: the server answers, the\n" +
			"clock agrees, the credential works, the home space is there, a file can be\n" +
			"written. A check stops the ones that depend on it. The output names providers,\n" +
			"expiry times and paths, never a token or a password, so it can go into a\n" +
			"support request. The write check puts one small file in the home space and\n" +
			"removes it: --skip-write leaves it out.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := runDoctor(cmd.Context(), rt, skipWrite)
			t := cli.Table{Headers: []string{"CHECK", "STATUS", "DETAIL", "REMEDY"}, Items: rep}
			for _, c := range rep.Checks {
				t.Rows = append(t.Rows, []string{c.Name, c.Status, c.Detail, c.Remedy})
			}
			if err := rt.Print(cmd.OutOrStdout(), t, nil); err != nil {
				return err
			}
			if rep.Problems > 0 {
				return fmt.Errorf("doctor found %d problems", rep.Problems)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipWrite, "skip-write", false, "do not write a probe file")
	return cmd
}

func runDoctor(ctx context.Context, rt *cli.Runtime, skipWrite bool) doctorReport {
	var rep doctorReport
	msgs := rt.Brand().Messages
	support := ""
	if u := rt.Brand().Program.SupportURL; u != "" {
		support = " (" + u + ")"
	}
	b, err := rt.Backend(ctx)
	if err != nil {
		rep.add("connection", "problem", err.Error(), "set an endpoint or a gateway: `"+rt.Name()+" config set endpoint https://<host>`")
		return rep
	}
	server := cmp.Or(rt.HTTPEndpoint(), rt.GatewayAddr())
	t, err := b.ServerTime(ctx)
	switch {
	case err == nil:
		detail := strings.TrimSpace(b.Transport() + " " + server)
		if d, err := rt.Data(ctx); err == nil {
			if c, err := d.Capabilities(ctx); err == nil && c.ServerVersion != "" {
				detail += ", " + strings.TrimSpace(c.ServerProduct+" "+c.ServerVersion)
			}
		}
		rep.add("server", "ok", detail, "")
		skew := time.Since(t)
		switch {
		case skew.Abs() >= 5*time.Minute:
			rep.add("clock", "problem", fmt.Sprintf("%s off the server", skew.Round(time.Second)), "Kerberos refuses a ticket more than five minutes out of step: set the clock of this machine")
		case skew.Abs() >= time.Minute:
			rep.add("clock", "warning", fmt.Sprintf("%s off the server", skew.Round(time.Second)), "credentials start to fail when this passes five minutes")
		default:
			rep.add("clock", "ok", "in step with the server", "")
		}
	case errors.Is(err, backend.ErrNotAvailable):
		rep.add("server", "ok", strings.TrimSpace(b.Transport()+" "+server), "")
		rep.add("clock", "skipped", "the "+b.Transport()+" transport gives no server time", "")
	default:
		rep.add("server", "problem", server+": "+err.Error(), cmp.Or(msgs.NetworkHint, "check the address, and the network of this machine"))
		return rep
	}
	tok, err := rt.Token(ctx)
	if err != nil {
		rep.add("credentials", "problem", err.Error(), "run `"+rt.Name()+" login`")
		return rep
	}
	detail := "a token from --token or its env"
	if s := rt.SignedIn(); s != nil {
		detail = s.Provider
	}
	if exp := cli.TokenExpiry(tok); !exp.IsZero() {
		detail += ", expires " + exp.Local().Format(time.RFC3339)
	}
	rep.add("credentials", "ok", detail, "")
	me, err := b.Users().Me(ctx)
	if err != nil {
		rep.add("identity", "problem", err.Error(), "the server does not say whose credential this is: `"+rt.Name()+" logout`, then sign in again")
		return rep
	}
	rep.add("identity", "ok", strings.TrimSpace(me.Username+" "+parens(me.DisplayName)), "")
	spaces, err := b.Spaces().List(ctx)
	if err != nil {
		rep.add("spaces", "problem", err.Error(), "")
		return rep
	}
	home, _ := backend.FindSpace(spaces, "home")
	if home == nil {
		rep.add("spaces", "problem", fmt.Sprintf("%d reachable, none of them yours", len(spaces)),
			cmp.Or(msgs.NoHomeHint, "your home space is not there: ask the support to create it")+support)
		return rep
	}
	root := home.GetRootInfo().GetPath()
	rep.add("spaces", "ok", fmt.Sprintf("%d reachable, home at %s", len(spaces), root), "")
	if q, err := b.Spaces().Quota(ctx, &provider.Reference{Path: root}); err == nil && q.Total > 0 {
		pct := q.Used * 100 / q.Total
		switch {
		case q.Remaining == 0:
			rep.add("quota", "problem", fmt.Sprintf("%d%% used", pct), "nothing can be written until files are deleted and purged from the trash: `"+rt.Name()+" du --top 20 home:` shows where the space went")
		case pct >= 90:
			rep.add("quota", "warning", fmt.Sprintf("%d%% used", pct), "`"+rt.Name()+" du --top 20 home:` shows where the space went")
		default:
			rep.add("quota", "ok", fmt.Sprintf("%d%% used", pct), "")
		}
	}
	if skipWrite {
		rep.add("write", "skipped", "--skip-write", "")
		return rep
	}
	d, err := rt.Data(ctx)
	if err != nil {
		rep.add("write", "problem", err.Error(), "")
		return rep
	}
	probe := path.Join(root, fmt.Sprintf(".%s-doctor-%d", rt.Name(), time.Now().UnixNano()))
	body := rt.Name() + " doctor: write probe\n"
	if err := d.UploadStream(ctx, pathRef(probe), strings.NewReader(body), int64(len(body))); err != nil {
		rep.add("write", "problem", err.Error(), "the storage did not take a small file: the space may be full, or read-only for you")
		return rep
	}
	if err := b.Files().Remove(ctx, pathRef(probe)); err != nil {
		rep.add("write", "warning", "the probe "+probe+" was written, but not removed: "+err.Error(), "remove it by hand")
		return rep
	}
	rep.add("write", "ok", fmt.Sprintf("wrote and removed %d bytes in %s", len(body), root), "")
	return rep
}

func parens(s string) string {
	if s == "" {
		return ""
	}
	return "(" + s + ")"
}
