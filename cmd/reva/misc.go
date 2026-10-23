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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCommand(rt *cli.Runtime) *cobra.Command {
	var list bool
	var username string
	cmd := &cobra.Command{
		Use:   "login <type>",
		Short: "Log in to the gateway and store the token",
		Long: "Log in to the gateway and store the token.\n\n" +
			"The password (or the API key for machine auth) is read from the terminal,\n" +
			"or from stdin when stdin is not a terminal:\n\n" +
			"  echo \"$PASSWORD\" | reva login --username alice basic",
		Args: func(cmd *cobra.Command, args []string) error {
			if list {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := rt.Client()
			if err != nil {
				return err
			}
			if list {
				res, err := client.ListAuthProviders(cmd.Context(), &registry.ListAuthProvidersRequest{})
				if err != nil {
					return err
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					return formatError(res.Status)
				}
				return rt.Print(cmd.OutOrStdout(), res.Types, func(w io.Writer) {
					for _, t := range res.Types {
						fmt.Fprintln(w, t)
					}
				})
			}

			username, secret, err := readCredentials(cmd, username)
			if err != nil {
				return err
			}
			res, err := client.Authenticate(cmd.Context(), &gateway.AuthenticateRequest{
				Type:         args[0],
				ClientId:     username,
				ClientSecret: secret,
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			if err := rt.SetToken(res.Token); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "list the available login types")
	cmd.Flags().StringVar(&username, "username", "", "the user name (default: ask)")
	return cmd
}

// readCredentials asks for the missing user name and for the secret. The
// secret is never a flag, so it does not show in ps or in the shell history.
func readCredentials(cmd *cobra.Command, username string) (string, string, error) {
	r := bufio.NewReader(cmd.InOrStdin())
	f, isFile := cmd.InOrStdin().(*os.File)
	tty := isFile && term.IsTerminal(int(f.Fd()))
	readLine := func(prompt string) (string, error) {
		if tty {
			fmt.Fprint(cmd.ErrOrStderr(), prompt)
		}
		line, err := r.ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return "", fmt.Errorf("reading %s: %w", strings.TrimSuffix(prompt, ": "), err)
		}
		return strings.TrimSpace(line), nil
	}
	var err error
	if username == "" {
		if username, err = readLine("username: "); err != nil {
			return "", "", err
		}
	}
	if !tty {
		secret, err := readLine("password: ")
		return username, secret, err
	}
	fmt.Fprint(cmd.ErrOrStderr(), "password: ")
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(cmd.ErrOrStderr())
	return username, strings.TrimSpace(string(b)), err
}

func whoamiCommand(rt *cli.Runtime) *cobra.Command {
	var token string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the user of the login token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := rt.Client()
			if err != nil {
				return err
			}
			if token == "" {
				if token, err = rt.Token(); err != nil {
					return err
				}
			}
			res, err := client.WhoAmI(cli.WithToken(cmd.Context(), token), &gateway.WhoAmIRequest{Token: token})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.User, func(w io.Writer) { fmt.Fprintln(w, res.User) })
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "the token to check (default: the login token)")
	return cmd
}

func configCommand(rt *cli.Runtime) *cobra.Command {
	keys := []string{"host", "admin-host"}
	cmd := &cobra.Command{Use: "config", Short: "Show or change the stored settings"}
	cmd.AddCommand(
		&cobra.Command{
			Use:       "set <key> <value>",
			Short:     "Store a setting (" + strings.Join(keys, ", ") + ")",
			Args:      cobra.ExactArgs(2),
			ValidArgs: keys,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := cli.ReadConfig()
				if err != nil {
					return err
				}
				switch args[0] {
				case "host":
					c.Host = args[1]
				case "admin-host":
					c.AdminHost = args[1]
				default:
					return fmt.Errorf("unknown key %q: use one of %s", args[0], strings.Join(keys, ", "))
				}
				return cli.WriteConfig(c)
			},
		},
		&cobra.Command{
			Use:   "view",
			Short: "Show the stored settings and where they are",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := cli.ReadConfig()
				if err != nil {
					return err
				}
				p, err := cli.ConfigPath()
				if err != nil {
					return err
				}
				return rt.Print(cmd.OutOrStdout(), c, func(w io.Writer) {
					fmt.Fprintf(w, "file:       %s\nhost:       %s\nadmin-host: %s\n", p, c.Host, c.AdminHost)
				})
			},
		},
	)
	return cmd
}

func openCommand(rt *cli.Runtime) *cobra.Command {
	var viewMode, app string
	var appInsecure, appSkipVerify bool
	cmd := &cobra.Command{
		Use:               "open <path>",
		Short:             "Open a file in an app and print the app URL",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			opaque := &types.Opaque{Map: map[string]*types.OpaqueEntry{}}
			if appInsecure {
				opaque.Map["insecure"] = &types.OpaqueEntry{}
			}
			if appSkipVerify {
				opaque.Map["skip-verify"] = &types.OpaqueEntry{}
			}
			res, err := client.OpenInApp(ctx, &gateway.OpenInAppRequest{
				Ref:      &provider.Reference{Path: args[0]},
				ViewMode: utils.GetViewMode(viewMode),
				App:      app,
				Opaque:   opaque,
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.AppUrl, func(w io.Writer) { fmt.Fprintf(w, "App URL: %+v\n", res.AppUrl) })
		},
	}
	cmd.Flags().StringVar(&viewMode, "viewmode", "view", "view mode: view, read or write")
	cmd.Flags().StringVar(&app, "app", "", "the app to use (default: the default app for the mime type)")
	cmd.Flags().BoolVar(&appInsecure, "app-insecure", false, "let the app provider connect to the app without transport security")
	cmd.Flags().BoolVar(&appSkipVerify, "app-skip-verify", false, "let the app provider skip the certificate check of the app")
	return cmd
}

func appPasswordCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "app-password", Short: "Manage app passwords"}
	cmd.AddCommand(appPasswordCreateCommand(rt), appPasswordListCommand(rt), appPasswordRemoveCommand(rt))
	return cmd
}

func appPasswordCreateCommand(rt *cli.Runtime) *cobra.Command {
	var label, expiration string
	var paths, shares []string
	var unlimited bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an app password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(paths) == 0 && len(shares) == 0 && !unlimited {
				return errors.New("give a scope: --path, --share or --all")
			}
			var exp *types.Timestamp
			if expiration != "" {
				t, err := time.Parse(time.DateOnly, expiration)
				if err != nil {
					return err
				}
				exp = &types.Timestamp{Seconds: uint64(t.Unix())}
			}
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			scopes, err := appPasswordScope(ctx, client, paths, shares, unlimited)
			if err != nil {
				return err
			}
			res, err := client.GenerateAppPassword(ctx, &authapp.GenerateAppPasswordRequest{
				Expiration: exp,
				Label:      label,
				TokenScope: scopes,
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.AppPassword, func(w io.Writer) {
				renderAppPasswords(w, []*authapp.AppPassword{res.AppPassword})
			})
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "label of the app password")
	cmd.Flags().StringVar(&expiration, "expiration", "", "expiration date (YYYY-MM-DD)")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "give access to a path, as path:r or path:w (repeatable)")
	cmd.Flags().StringArrayVar(&shares, "share", nil, "give access to a public link, as share_id:r or share_id:w (repeatable)")
	cmd.Flags().BoolVar(&unlimited, "all", false, "give access to everything the user can access")
	return cmd
}

func appPasswordScope(ctx context.Context, client gateway.GatewayAPIClient, paths, shares []string, unlimited bool) (map[string]*authpb.Scope, error) {
	if unlimited {
		return scope.AddOwnerScope(nil)
	}
	var scopes map[string]*authpb.Scope
	for _, entry := range shares {
		id, role, err := splitPermission(entry)
		if err != nil {
			return nil, err
		}
		res, err := client.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: publicShareRef(id)})
		if err != nil {
			return nil, err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return nil, formatError(res.Status)
		}
		if scopes, err = scope.AddPublicShareScope(res.Share, role, scopes); err != nil {
			return nil, err
		}
	}
	for _, entry := range paths {
		p, role, err := splitPermission(entry)
		if err != nil {
			return nil, err
		}
		res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: p}})
		if err != nil {
			return nil, err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return nil, formatError(res.Status)
		}
		if scopes, err = scope.AddResourceInfoScope(res.Info, role, scopes); err != nil {
			return nil, err
		}
	}
	return scopes, nil
}

// splitPermission splits "<value>:r" or "<value>:w". It cuts at the last
// colon, so the value itself can have colons.
func splitPermission(entry string) (string, authpb.Role, error) {
	i := strings.LastIndex(entry, ":")
	if i < 0 {
		return "", authpb.Role_ROLE_INVALID, fmt.Errorf("invalid scope %q: use <value>:r or <value>:w", entry)
	}
	switch entry[i+1:] {
	case "r":
		return entry[:i], authpb.Role_ROLE_VIEWER, nil
	case "w":
		return entry[:i], authpb.Role_ROLE_EDITOR, nil
	}
	return "", authpb.Role_ROLE_INVALID, fmt.Errorf("invalid permission in %q: use r or w", entry)
}

func appPasswordListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your app passwords",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListAppPasswords(ctx, &authapp.ListAppPasswordsRequest{})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.AppPasswords, func(w io.Writer) { renderAppPasswords(w, res.AppPasswords) })
		},
	}
}

func appPasswordRemoveCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <password>",
		Short: "Invalidate an app password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.InvalidateAppPassword(ctx, &authapp.InvalidateAppPasswordRequest{Password: args[0]})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
}

func renderAppPasswords(w io.Writer, pws []*authapp.AppPassword) {
	formatTime := func(t *types.Timestamp) string {
		if t == nil {
			return ""
		}
		return time.Unix(int64(t.Seconds), 0).String()
	}
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{"Token", "Scope", "Label", "Expiration", "Creation Time", "Last Used Time"})
	for _, pw := range pws {
		var scopes []string
		for k, s := range pw.TokenScope {
			f, err := scope.FormatScope(k, s)
			if err != nil {
				f = k
			}
			scopes = append(scopes, f)
		}
		t.AppendRow(table.Row{pw.Password, strings.Join(scopes, ", "), pw.Label, formatTime(pw.Expiration), formatTime(pw.Ctime), formatTime(pw.Utime)})
	}
	t.Render()
}

func preferencesCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "preferences", Short: "Get and set user preferences"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get <namespace> <key>",
			Short: "Show a preference",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				client, ctx, err := rt.Gateway(cmd.Context())
				if err != nil {
					return err
				}
				res, err := client.GetKey(ctx, &preferences.GetKeyRequest{Key: &preferences.PreferenceKey{Namespace: args[0], Key: args[1]}})
				if err != nil {
					return err
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					return formatError(res.Status)
				}
				return rt.Print(cmd.OutOrStdout(), res.Val, func(w io.Writer) { fmt.Fprintln(w, res.Val) })
			},
		},
		&cobra.Command{
			Use:   "set <namespace> <key> <value>",
			Short: "Set a preference",
			Args:  cobra.ExactArgs(3),
			RunE: func(cmd *cobra.Command, args []string) error {
				client, ctx, err := rt.Gateway(cmd.Context())
				if err != nil {
					return err
				}
				res, err := client.SetKey(ctx, &preferences.SetKeyRequest{
					Key: &preferences.PreferenceKey{Namespace: args[0], Key: args[1]},
					Val: args[2],
				})
				if err != nil {
					return err
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					return formatError(res.Status)
				}
				return nil
			},
		},
	)
	return cmd
}
