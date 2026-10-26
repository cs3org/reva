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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCommand(rt *cli.Runtime) *cobra.Command {
	var list bool
	var method, spn, ccache string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to the gateway and store the token",
		Long: "Sign in to the gateway and store the token.\n\n" +
			"--method is the auth type of the gateway: basic by default, appauth with\n" +
			"--app-token-file. 'reva login --list' shows the types. The password (or the\n" +
			"API key for machine auth) is read from the terminal, or from stdin when\n" +
			"stdin is not a terminal:\n\n" +
			"  echo \"$PASSWORD\" | reva login --user alice\n\n" +
			"--method kerberos uses the ticket of kinit and asks for no password. The\n" +
			"ticket is read from --ccache, else $KRB5CCNAME, else /tmp/krb5cc_<uid>; only\n" +
			"FILE: caches can be read. The service principal is --spn, else the kerberos_spn\n" +
			"setting ('reva config set kerberos-spn'), else HTTP/<gateway host>:\n\n" +
			"  kinit && reva login --method kerberos --spn HTTP/cernbox.cern.ch",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				client, err := rt.Client()
				if err != nil {
					return err
				}
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

			if method == "kerberos" {
				return kerberosLogin(cmd, rt, spn, ccache)
			}
			if cmd.Flags().Changed("spn") || cmd.Flags().Changed("ccache") {
				return errors.New("--spn and --ccache apply only to --method kerberos")
			}

			var user, secret string
			var err error
			if rt.AppTokenFile() != "" {
				if !cmd.Flags().Changed("method") {
					method = rt.AuthType("app-token")
				}
				b, err := os.ReadFile(rt.AppTokenFile())
				if err != nil {
					return err
				}
				if rt.User() == "" {
					return errors.New("an app token needs a user name: pass --user")
				}
				user, secret = rt.User(), strings.TrimSpace(string(b))
			} else if rt.Password() != "" {
				if rt.User() == "" {
					return errors.New("a password from the environment needs a user name: pass --user")
				}
				user, secret = rt.User(), rt.Password()
			} else if user, secret, err = readCredentials(cmd, rt.User()); err != nil {
				return err
			}
			token, err := rt.Authenticate(cmd.Context(), method, user, secret)
			if err != nil {
				return err
			}
			if err := state.SetToken(token); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Signed in as %s using %s.", user, method)
			return nil
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "list the auth types of the gateway")
	cmd.Flags().StringVar(&method, "method", "basic", "auth type to sign in with")
	cmd.Flags().StringVar(&spn, "spn", "", "service principal for --method kerberos, for example HTTP/cernbox.cern.ch")
	cmd.Flags().StringVar(&ccache, "ccache", "", "Kerberos credential cache for --method kerberos (default $KRB5CCNAME, then /tmp/krb5cc_<uid>)")
	_ = cmd.RegisterFlagCompletionFunc("spn", cobra.NoFileCompletions)
	_ = cmd.RegisterFlagCompletionFunc("method", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if err := rt.Load(cmd.Flags()); err != nil {
			return nil, none
		}
		client, err := rt.Client()
		if err != nil {
			return nil, none
		}
		res, err := client.ListAuthProviders(cmd.Context(), &registry.ListAuthProvidersRequest{})
		if err != nil {
			return nil, none
		}
		return res.Types, none
	})
	return cmd
}

func logoutCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored tokens",
		Long:  "Forget the stored login token and the admin token of 'reva elevate'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := state.ClearTokens(); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Signed out.")
			return nil
		},
	}
}

type statusResult struct {
	Transport string `json:"transport"`
	Gateway   string `json:"gateway"`
	Endpoint  string `json:"endpoint,omitempty"`
	AdminHost string `json:"admin_host,omitempty"`
	Config    string `json:"config"`
	TokenFile string `json:"token_file"`
	User      string `json:"user,omitempty"`
	Expires   string `json:"expires,omitempty"`
	Version   string `json:"client_version"`
	Error     string `json:"error,omitempty"`
}

func statusCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show how you are signed in",
		Long: "Show the gateway, how you are signed in, and where the settings and the\n" +
			"token are kept. Run this first when something does not work.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st := statusResult{Transport: rt.Transport(), Gateway: rt.GatewayAddr(), Endpoint: rt.HTTPEndpoint(), AdminHost: rt.AdminHost(), Version: version}
			st.Config, _ = state.ConfigPath()
			st.TokenFile, _ = state.TokenPath()
			user, exp, err := whoami(cmd.Context(), rt)
			if err != nil {
				st.Error = err.Error()
			} else {
				st.User = user.GetUsername()
				if !exp.IsZero() {
					st.Expires = exp.Local().Format(time.RFC3339) + " (in " + time.Until(exp).Round(time.Second).String() + ")"
				}
			}
			return rt.Print(cmd.OutOrStdout(), st, func(w io.Writer) {
				dash := func(s string) string {
					if s == "" {
						return "-"
					}
					return s
				}
				fmt.Fprintf(w, "Transport:   %s\n", st.Transport)
				fmt.Fprintf(w, "Gateway:     %s\n", dash(st.Gateway))
				fmt.Fprintf(w, "Endpoint:    %s\n", dash(st.Endpoint))
				fmt.Fprintf(w, "Admin host:  %s\n", dash(st.AdminHost))
				fmt.Fprintf(w, "User:        %s\n", dash(st.User))
				fmt.Fprintf(w, "Expires:     %s\n", dash(st.Expires))
				fmt.Fprintf(w, "Config:      %s\n", st.Config)
				fmt.Fprintf(w, "Token file:  %s\n", st.TokenFile)
				fmt.Fprintf(w, "Client:      %s\n", dash(st.Version))
				if st.Error != "" {
					fmt.Fprintf(w, "Problem:     %s\n", st.Error)
				}
			})
		},
	}
}

// whoami returns the user of the token and when the token expires.
func whoami(ctx context.Context, rt *cli.Runtime) (*userpb.User, time.Time, error) {
	b, err := rt.Backend(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	user, err := b.Users().Me(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	token, err := rt.Token(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	return user, cli.TokenExpiry(token), nil
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
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show who you are signed in as, and your admin rights",
		Long: "Show who you are signed in as, and your admin rights: socket (the local\n" +
			"admin socket), yes with the time left on the admin token, or no. With --as,\n" +
			"show the user you impersonate.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			user, _, err := whoami(cmd.Context(), rt)
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), user, func(w io.Writer) {
				fmt.Fprintln(w, user)
				if rt.As() != "" {
					fmt.Fprintf(w, "as: %s (impersonated)\n", user.GetUsername())
				}
				fmt.Fprintf(w, "admin: %s\n", rt.AdminState(cmd.Context()))
			})
		},
	}
}

func configCommand(rt *cli.Runtime) *cobra.Command {
	keys := []string{"gateway", "admin-host", "endpoint", "transport", "kerberos-spn", "output", "operator-commands"}
	cmd := &cobra.Command{Use: "config", Short: "Show or change the stored settings"}
	cmd.AddCommand(
		&cobra.Command{
			Use:       "set KEY VALUE",
			Short:     "Store a setting (" + strings.Join(keys, ", ") + ")",
			Args:      cobra.ExactArgs(2),
			ValidArgs: keys,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := state.ReadConfig()
				if err != nil {
					return err
				}
				oneOf := func(dst *string, valid []string) error {
					if !slices.Contains(valid, args[1]) {
						return fmt.Errorf("%s %q: use one of %s", args[0], args[1], strings.Join(valid, ", "))
					}
					*dst = args[1]
					return nil
				}
				switch args[0] {
				case "gateway":
					c.Connection.Gateway = args[1]
				case "admin-host":
					c.Connection.AdminHost = args[1]
				case "endpoint":
					c.Connection.Endpoint = args[1]
				case "transport":
					err = oneOf(&c.Connection.Transport, brand.Transports)
				case "kerberos-spn":
					c.Auth.Kerberos.SPN = args[1]
				case "output":
					err = oneOf(&c.UI.Output, brand.Outputs)
				case "operator-commands":
					err = oneOf(&c.UI.OperatorCommands, brand.OperatorModes)
				default:
					return fmt.Errorf("unknown key %q: use one of %s", args[0], strings.Join(keys, ", "))
				}
				if err != nil {
					return err
				}
				return state.WriteConfig(c)
			},
		},
		&cobra.Command{
			Use:   "view",
			Short: "Show the settings and where they come from",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				p, err := state.ConfigPath()
				if err != nil {
					return err
				}
				type row struct {
					Key   string `json:"key"`
					Value string `json:"value"`
					From  string `json:"from"`
				}
				v := struct {
					File       string `json:"file"`
					SystemFile string `json:"system_file"`
					Settings   []row  `json:"settings"`
				}{File: p, SystemFile: state.SystemConfigPath()}
				for _, k := range keys {
					val, from := rt.Setting(strings.ReplaceAll(k, "-", "_"))
					v.Settings = append(v.Settings, row{k, val, from})
				}
				return rt.Print(cmd.OutOrStdout(), v, func(w io.Writer) {
					fmt.Fprintf(w, "file:              %s\nsystem file:       %s\n", v.File, v.SystemFile)
					for _, r := range v.Settings {
						if r.Value == "" {
							r.Value = "-"
						}
						fmt.Fprintf(w, "%-18s %s (%s)\n", r.Key+":", r.Value, r.From)
					}
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
		Use:               "open PATH",
		Short:             "Open a file in an app and print the app URL",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := rt.Data(cmd.Context())
			if err != nil {
				return err
			}
			appURL, err := d.OpenInApp(cmd.Context(), pathRef(args[0]), backend.AppOptions{
				App:        app,
				ViewMode:   utils.GetViewMode(viewMode),
				Insecure:   appInsecure,
				SkipVerify: appSkipVerify,
			})
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), appURL, func(w io.Writer) { fmt.Fprintf(w, "App URL: %+v\n", appURL) })
		},
	}
	cmd.Flags().StringVar(&viewMode, "view-mode", "view", "view mode: view, read or write")
	cmd.Flags().StringVar(&app, "app", "", "the app to use (default: the default app for the mime type)")
	cmd.Flags().BoolVar(&appInsecure, "app-insecure", false, "let the app provider connect to the app without transport security")
	cmd.Flags().BoolVar(&appSkipVerify, "app-skip-verify", false, "let the app provider skip the certificate check of the app")
	return cmd
}

func tokenCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage app tokens for scripts and scheduled jobs",
		Long: "App tokens are long-lived credentials for scripts and scheduled jobs.\n\n" +
			"Keep them narrow. A token limited to one path and read-only does far less\n" +
			"damage if it leaks than one with full access to your account.",
	}
	cmd.AddCommand(tokenListCommand(rt), tokenRevokeCommand(rt), tokenCreateCommand(rt))
	return cmd
}

func tokenCreateCommand(rt *cli.Runtime) *cobra.Command {
	var label, expiry, permission string
	var paths, shares []string
	var unlimited bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an app token",
		Example: "  reva token create --path /home/data --permission read --expiry 2026-12-31\n" +
			"  reva token create --all --label 'nightly backup'",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(paths) == 0 && len(shares) == 0 && !unlimited {
				return errors.New("pass --path or --share to limit the token, or --all to give it full access to your account")
			}
			var role authpb.Role
			switch permission {
			case "read":
				role = authpb.Role_ROLE_VIEWER
			case "write":
				role = authpb.Role_ROLE_EDITOR
			default:
				return fmt.Errorf("unknown permission %q: use read or write", permission)
			}
			var exp *types.Timestamp
			if expiry != "" {
				t, err := time.Parse(time.DateOnly, expiry)
				if err != nil {
					return fmt.Errorf("invalid expiry %q: use a date like 2026-12-31", expiry)
				}
				// The end of the day, so that the token works during all of that date.
				exp = &types.Timestamp{Seconds: uint64(t.Add(24*time.Hour - time.Second).Unix())}
			}
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			scopes, err := tokenScope(ctx, b, paths, shares, role, unlimited)
			if err != nil {
				return err
			}
			pw, err := b.Tokens().Create(ctx, &authapp.GenerateAppPasswordRequest{
				Expiration: exp,
				Label:      label,
				TokenScope: scopes,
			})
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), pw, func(w io.Writer) {
				renderAppPasswords(w, "Token", []*authapp.AppPassword{pw})
			})
		},
	}
	cmd.Flags().StringArrayVar(&paths, "path", nil, "limit the token to this path (repeatable)")
	cmd.Flags().StringArrayVar(&shares, "share", nil, "limit the token to this public link id (repeatable)")
	cmd.Flags().StringVar(&permission, "permission", "read", "read or write")
	cmd.Flags().StringVar(&expiry, "expiry", "", "expiry date, YYYY-MM-DD")
	cmd.Flags().StringVar(&label, "label", "", "label to identify the token later")
	cmd.Flags().BoolVar(&unlimited, "all", false, "create an unscoped token with full account access")
	return cmd
}

func tokenScope(ctx context.Context, b backend.Backend, paths, shares []string, role authpb.Role, unlimited bool) (map[string]*authpb.Scope, error) {
	if unlimited {
		return scope.AddOwnerScope(nil)
	}
	var scopes map[string]*authpb.Scope
	for _, id := range shares {
		s, err := b.Links().Get(ctx, publicShareRef(id))
		if err != nil {
			return nil, err
		}
		if scopes, err = scope.AddPublicShareScope(s, role, scopes); err != nil {
			return nil, err
		}
	}
	for _, p := range paths {
		info, err := b.Files().Stat(ctx, pathRef(p))
		if err != nil {
			return nil, err
		}
		if scopes, err = scope.AddResourceInfoScope(info, role, scopes); err != nil {
			return nil, err
		}
	}
	return scopes, nil
}

func tokenListCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the app tokens on your account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			pws, err := b.Tokens().List(cmd.Context())
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), pws, func(w io.Writer) { renderAppPasswords(w, "ID", pws) })
		},
	}
}

func tokenRevokeCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke ID",
		Short: "Revoke an app token",
		Long:  "Revoke an app token. The ID is the one that 'reva token list' shows.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: completeIDs(rt, func(ctx context.Context, b backend.Backend) ([]string, error) {
			pws, err := b.Tokens().List(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(pws))
			for _, pw := range pws {
				ids = append(ids, pw.Password+"\t"+pw.Label)
			}
			return ids, nil
		}),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			if err := b.Tokens().Revoke(cmd.Context(), args[0]); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Revoked app token %s", args[0])
			return nil
		},
	}
}

// renderAppPasswords prints the tokens. The first column is the id in a
// listing, and the secret right after create, the only time the server
// returns it.
func renderAppPasswords(w io.Writer, first string, pws []*authapp.AppPassword) {
	formatTime := func(t *types.Timestamp) string {
		if t == nil {
			return ""
		}
		return time.Unix(int64(t.Seconds), 0).String()
	}
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row{first, "Scope", "Label", "Expiration", "Creation Time", "Last Used Time"})
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
			Use:   "get NAMESPACE KEY",
			Short: "Show a preference",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				p, err := cli.Ability[backend.PreferenceStore](cmd.Context(), rt, "preferences")
				if err != nil {
					return err
				}
				val, err := p.GetPreference(cmd.Context(), args[0], args[1])
				if err != nil {
					return err
				}
				return rt.Print(cmd.OutOrStdout(), val, func(w io.Writer) { fmt.Fprintln(w, val) })
			},
		},
		&cobra.Command{
			Use:   "set NAMESPACE KEY VALUE",
			Short: "Set a preference",
			Args:  cobra.ExactArgs(3),
			RunE: func(cmd *cobra.Command, args []string) error {
				p, err := cli.Ability[backend.PreferenceStore](cmd.Context(), rt, "preferences")
				if err != nil {
					return err
				}
				return p.SetPreference(cmd.Context(), args[0], args[1], args[2])
			},
		},
	)
	return cmd
}
