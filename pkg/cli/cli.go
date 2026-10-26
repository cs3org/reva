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

// Package cli holds the state that all reva commands share: the settings,
// the tokens and the connections. It is also the API of the CLI plugins, so
// what it exports must stay stable.
package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/spf13/pflag"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Runtime is built once per process from the flags, the environment and the
// config file, in this order of precedence. It is the API of CLI plugins, so
// its settings are read through accessors.
type Runtime struct {
	gatewayAddr  string
	adminHost    string
	insecure     bool
	skipVerify   bool
	tokenValue   string
	appTokenFile string
	user         string
	output       string
	quiet        bool
	debug        bool
	noProgress   bool
	timeout      time.Duration
	// as is the user of --as: the user plane runs with an impersonated token.
	as string
	// reason of --reason, sent with every admin-plane call for the audit.
	reason    string
	yes       bool
	noElevate bool
	// httpEndpoint is the HTTPS endpoint of the deployment (--endpoint), for
	// the HTTPS transport, and for --as when the Admin API cannot be reached.
	httpEndpoint string
	config       state.Config

	conn              *grpc.ClientConn
	admin             *AdminClient
	adminHostExplicit bool
	adminTok          string
	asTok             string
	plane             Plane
	need              string
	usedAdmin         bool
	transport         string
	backend           backend.Backend
	data              backend.DataPlane
}

// AddFlags adds the global flags, which set the runtime, to fs.
func (r *Runtime) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&r.gatewayAddr, "gateway", "", "address of the gateway, host:port (env REVA_GATEWAY)")
	fs.StringVar(&r.httpEndpoint, "endpoint", "", "HTTPS endpoint of the deployment, https://host (env REVA_ENDPOINT)")
	fs.StringVar(&r.transport, "transport", "", "transport of the user plane: grpc, http or auto (env REVA_TRANSPORT; default "+DefaultTransport+")")
	fs.StringVar(&r.adminHost, "admin-host", "", "address of the Admin API, host:port or unix:///path (env REVA_ADMIN_HOST; default the gateway)")
	fs.BoolVar(&r.insecure, "insecure", false, "allow plaintext, without TLS, on the selected transport (development only)")
	fs.BoolVar(&r.skipVerify, "skip-verify", false, "do not verify the server certificate (development only)")
	fs.StringVar(&r.tokenValue, "token", "", "use this token instead of signing in (env REVA_TOKEN)")
	fs.StringVar(&r.appTokenFile, "app-token-file", "", "file holding an app token, to sign in with --user")
	fs.StringVar(&r.user, "user", "", "user name, for app-token and basic sign-in")
	fs.StringVar(&r.as, "as", "", "run the command as this user (impersonation, needs admin rights)")
	fs.StringVar(&r.reason, "reason", "", "why you use admin rights, written to the audit log")
	fs.BoolVarP(&r.yes, "yes", "y", false, "do not ask for confirmations")
	fs.BoolVar(&r.noElevate, "no-elevate", false, "never get an admin token automatically (env REVA_NO_ELEVATE)")
	fs.DurationVar(&r.timeout, "timeout", 0, "overall timeout for the command, for example 30s (0 for none)")
	fs.StringVarP(&r.output, "output", "o", "table", "output format: table or json")
	fs.BoolVarP(&r.quiet, "quiet", "q", false, "hide progress messages")
	fs.BoolVar(&r.debug, "debug", false, "show the underlying error and request details")
	fs.BoolVar(&r.noProgress, "no-progress", false, "hide the progress bar")
}

// GatewayAddr returns the address of the gateway.
func (r *Runtime) GatewayAddr() string { return r.gatewayAddr }

// AdminHost returns the address of the Admin API. Empty means the gateway.
func (r *Runtime) AdminHost() string { return r.adminHost }

// User returns the user name of --user.
func (r *Runtime) User() string { return r.user }

// AppTokenFile returns the file of --app-token-file.
func (r *Runtime) AppTokenFile() string { return r.appTokenFile }

// As returns the user of --as. Empty means no impersonation.
func (r *Runtime) As() string { return r.as }

// Reason returns the reason of --reason, for the audit.
func (r *Runtime) Reason() string { return r.reason }

// Output returns the output format: table or json.
func (r *Runtime) Output() string { return r.output }

// Debug reports whether --debug is set.
func (r *Runtime) Debug() bool { return r.debug }

// Timeout returns the overall timeout of the command. Zero means none.
func (r *Runtime) Timeout() time.Duration { return r.timeout }

// Load fills the settings that are not given as flags from the environment,
// then from the config file.
func (r *Runtime) Load(flags *pflag.FlagSet) error {
	c, err := state.ReadConfig()
	if err != nil {
		return err
	}
	r.config = c
	fill := func(dst *string, flag, conf string, envs ...string) {
		if flags.Changed(flag) {
			return
		}
		for _, env := range envs {
			if v := os.Getenv(env); v != "" {
				*dst = v
				return
			}
		}
		*dst = conf
	}
	// REVA_HOST is the name from before --gateway, kept for old scripts.
	fill(&r.gatewayAddr, "gateway", c.Connection.Gateway, "REVA_GATEWAY", "REVA_HOST")
	fill(&r.adminHost, "admin-host", c.Connection.AdminHost, "REVA_ADMIN_HOST")
	r.adminHostExplicit = flags.Changed("admin-host") || os.Getenv("REVA_ADMIN_HOST") != ""
	if v, err := strconv.ParseBool(os.Getenv("REVA_NO_ELEVATE")); err == nil && !flags.Changed("no-elevate") {
		r.noElevate = v
	}
	fill(&r.tokenValue, "token", "", "REVA_TOKEN")
	if err := r.loadTransport(flags, c); err != nil {
		return err
	}
	switch r.output {
	case "table", "json":
	case "csv":
		return errors.New("csv output is not available yet: use table or json")
	default:
		return fmt.Errorf("unknown output %q: use table or json", r.output)
	}
	return nil
}

// Client returns a gateway client without credentials, for the calls that
// do not need a login (for example Authenticate).
func (r *Runtime) Client() (gateway.GatewayAPIClient, error) {
	if r.conn == nil {
		if r.gatewayAddr == "" {
			return nil, errors.New("gateway not set: pass --gateway, set REVA_GATEWAY or run `reva config set gateway <host:port>`")
		}
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: r.skipVerify})
		if r.insecure {
			creds = insecure.NewCredentials()
		}
		opts := append(r.DialOptions(), grpc.WithTransportCredentials(creds), gatewayDialOption())
		conn, err := grpc.NewClient(r.gatewayAddr, opts...)
		if err != nil {
			return nil, err
		}
		r.conn = conn
	}
	return gateway.NewGatewayAPIClient(r.conn), nil
}

// DialOptions are the options that every gRPC connection of the CLI shares.
func (r *Runtime) DialOptions() []grpc.DialOption {
	if !r.debug {
		return nil
	}
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			start := time.Now()
			err := invoker(ctx, method, req, reply, cc, opts...)
			r.Debugf("grpc %s %s %s", method, status.Code(err), time.Since(start).Round(time.Millisecond))
			return err
		}),
		grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			r.Debugf("grpc stream %s", method)
			return streamer(ctx, desc, cc, method, opts...)
		}),
	}
}

// Gateway returns a gateway client and ctx with the login token attached.
func (r *Runtime) Gateway(ctx context.Context) (gateway.GatewayAPIClient, context.Context, error) {
	client, err := r.Client()
	if err != nil {
		return nil, nil, err
	}
	t, err := r.Token(ctx)
	if err != nil {
		return nil, nil, err
	}
	return client, WithToken(ctx, t), nil
}

// Authenticate exchanges a credential for a reva token. authType is the name
// of the auth provider in the gateway, for example basic or appauth.
func (r *Runtime) Authenticate(ctx context.Context, authType, user, secret string) (string, error) {
	client, err := r.Client()
	if err != nil {
		return "", err
	}
	res, err := client.Authenticate(ctx, &gateway.AuthenticateRequest{Type: authType, ClientId: user, ClientSecret: secret})
	if err != nil {
		return "", err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return "", fmt.Errorf("sign-in with %s failed: code=%v msg=%q", authType, res.Status.Code, res.Status.Message)
	}
	return res.Token, nil
}

// WithToken attaches a reva token to ctx and to the outgoing gRPC metadata.
// It replaces a token that ctx already has, so that a user-plane context can
// become an admin-plane one.
func WithToken(ctx context.Context, t string) context.Context {
	ctx = appctx.ContextSetToken(ctx, t)
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(appctx.TokenHeader, t)
	return metadata.NewOutgoingContext(ctx, md)
}

// HTTPClient returns the client for the data transfers. It sends the token
// in the request context. The command context sets the deadline, so the
// client has no timeout of its own.
func (r *Runtime) HTTPClient() *httpclient.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: r.skipVerify}
	return httpclient.New(httpclient.RoundTripper(tr))
}

// Close releases the gateway and the admin connections.
func (r *Runtime) Close() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
	if r.admin != nil {
		_ = r.admin.Conn.Close()
	}
}

// Token returns the token of the user plane: the impersonated token of --as,
// else the login token. The impersonated token stays in memory.
func (r *Runtime) Token(ctx context.Context) (string, error) {
	if r.as == "" {
		return r.loginToken(ctx)
	}
	if r.asTok == "" {
		t, err := r.impersonate(ctx)
		if err != nil {
			return "", err
		}
		r.asTok = t
	}
	return r.asTok, nil
}

// loginToken returns the token from --token or REVA_TOKEN. Else it signs in
// with the app token of --app-token-file. Else it returns the token that
// `reva login` stored.
func (r *Runtime) loginToken(ctx context.Context) (string, error) {
	if r.tokenValue != "" {
		return r.tokenValue, nil
	}
	if r.appTokenFile != "" {
		b, err := os.ReadFile(r.appTokenFile)
		if err != nil {
			return "", err
		}
		if r.user == "" {
			return "", errors.New("an app token needs a user name: pass --user")
		}
		t, err := r.Authenticate(ctx, "appauth", r.user, strings.TrimSpace(string(b)))
		if err != nil {
			return "", err
		}
		r.tokenValue = t
		return t, nil
	}
	b, err := state.ReadToken()
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("no login token: run `reva login` first")
	}
	return strings.TrimSpace(string(b)), err
}

// UseColor reports whether to color the output written to f: only for a
// terminal, and never when NO_COLOR is set (https://no-color.org).
func UseColor(f *os.File) bool {
	return os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(f.Fd()))
}

// Msg writes a message for a person to stderr. -q and -o json hide it, so
// that only the result goes to a script.
func (r *Runtime) Msg(w io.Writer, format string, args ...any) {
	if r.quiet || r.output == "json" {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}

// Debugf writes a line to stderr with --debug.
func (r *Runtime) Debugf(format string, args ...any) {
	if r.debug {
		fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
	}
}

// ShowProgress reports whether to draw a progress bar: only for a person
// that watches stderr, and not with -q, --no-progress or -o json.
func (r *Runtime) ShowProgress() bool {
	return !r.quiet && !r.noProgress && r.output != "json" && term.IsTerminal(int(os.Stderr.Fd()))
}

// Print writes v as JSON with -o json. Else it calls text. For a Table, a nil
// text writes the table.
func (r *Runtime) Print(w io.Writer, v any, text func(w io.Writer)) error {
	if t, ok := v.(Table); ok {
		if text == nil {
			text = t.Write
		}
		v = t.items()
	}
	if r.output != "json" {
		text(w)
		return nil
	}
	b, err := marshalJSON(v)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return err
	}
	out.WriteByte('\n')
	_, err = out.WriteTo(w)
	return err
}

// marshalJSON uses protojson for proto messages, because encoding/json does
// not know the proto field names and enums. Slices are marshalled per item.
func marshalJSON(v any) ([]byte, error) {
	if m, ok := v.(proto.Message); ok {
		return protojson.Marshal(m)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		items := make([]json.RawMessage, rv.Len())
		for i := range rv.Len() {
			b, err := marshalJSON(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			items[i] = b
		}
		return json.Marshal(items)
	}
	return json.Marshal(v)
}
