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
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

// minQuery is the shortest query of find and of the completion, because a
// short query can match a large part of about 1M users.
const minQuery = 3

func userCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Look up users and their groups"}
	cmd.AddCommand(userGetCommand(rt), userFindCommand(rt), userGroupsCommand(rt))
	return cmd
}

func userGetCommand(rt *cli.Runtime) *cobra.Command {
	var claim string
	cmd := &cobra.Command{
		Use:   "get NAME",
		Short: "Show a user",
		Long: "Show a user. NAME is the username, or with --claim the mail or the uid\n" +
			"number. -o json also shows the groups of the user.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUsers(rt, &claim),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := lookupUser(cmd.Context(), rt, claim, args[0])
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), u, userTable([]*userpb.User{u}).Write)
		},
	}
	claimFlag(cmd, &claim)
	return cmd
}

func userFindCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "find QUERY",
		Short: "Find users by username, name or mail",
		Long: fmt.Sprintf("Find the users of every type whose username, name or mail matches QUERY.\n"+
			"QUERY has at least %d characters.", minQuery),
		Args: cobra.MatchAll(cobra.ExactArgs(1), func(cmd *cobra.Command, args []string) error {
			if len(args[0]) < minQuery {
				return fmt.Errorf("the query %q is too short: use at least %d characters", args[0], minQuery)
			}
			return nil
		}),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			users, err := b.Users().Find(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), userTable(users), nil)
		},
	}
}

func userGroupsCommand(rt *cli.Runtime) *cobra.Command {
	var claim string
	cmd := &cobra.Command{
		Use:   "groups NAME",
		Short: "List the groups of a user",
		Long: "List the groups of a user, one per line. NAME is the username, or with\n" +
			"--claim the mail or the uid number.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUsers(rt, &claim),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := lookupUser(cmd.Context(), rt, claim, args[0])
			if err != nil {
				return err
			}
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			groups, err := b.Users().Groups(cmd.Context(), u.Id)
			if err != nil {
				return err
			}
			slices.Sort(groups)
			return rt.Print(cmd.OutOrStdout(), groups, func(w io.Writer) {
				for _, g := range groups {
					fmt.Fprintln(w, g)
				}
			})
		},
	}
	claimFlag(cmd, &claim)
	return cmd
}

func claimFlag(cmd *cobra.Command, claim *string) {
	cmd.Flags().StringVar(claim, "claim", "username", "what NAME is: username, mail or uid")
	_ = cmd.RegisterFlagCompletionFunc("claim", cobra.FixedCompletions([]string{"username", "mail", "uid"}, cobra.ShellCompDirectiveNoFileComp))
}

func lookupUser(ctx context.Context, rt *cli.Runtime, claim, value string) (*userpb.User, error) {
	b, err := rt.Backend(ctx)
	if err != nil {
		return nil, err
	}
	return b.Users().ByClaim(ctx, claim, value)
}

// completeUsers completes NAME from find. find does not search the uid
// numbers, so a uid is not completed.
func completeUsers(rt *cli.Runtime, claim *string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if len(args) > 0 || len(toComplete) < minQuery || *claim == "uid" {
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
		users, err := b.Users().Find(cmd.Context(), toComplete)
		if err != nil {
			return nil, none
		}
		var out []string
		for _, u := range users {
			v := u.Username
			if *claim == "mail" {
				v = u.Mail
			}
			if v != "" && strings.HasPrefix(v, toComplete) {
				out = append(out, v+"\t"+u.DisplayName)
			}
		}
		return out, none
	}
}

func userTable(users []*userpb.User) cli.Table {
	t := cli.Table{Headers: []string{"Username", "Name", "Mail", "UID", "GID", "Type", "IdP"}, Items: users}
	for _, u := range users {
		t.Rows = append(t.Rows, []string{
			u.Username, u.DisplayName, u.Mail,
			strconv.FormatInt(u.UidNumber, 10), strconv.FormatInt(u.GidNumber, 10),
			strings.ToLower(strings.TrimPrefix(u.Id.GetType().String(), "USER_TYPE_")), u.Id.GetIdp(),
		})
	}
	return t
}
