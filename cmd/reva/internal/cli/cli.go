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
// the tokens and the gateway connection.
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
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
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
// config file, in this order of precedence.
type Runtime struct {
	GatewayAddr  string
	AdminHost    string
	Insecure     bool
	SkipVerify   bool
	TokenValue   string
	AppTokenFile string
	User         string
	Output       string
	Quiet        bool
	Debug        bool
	NoProgress   bool
	Timeout      time.Duration
	// As is the user of --as: the user plane runs with an impersonated token.
	As string
	// Reason of --reason, sent with every admin-plane call for the audit.
	Reason    string
	Yes       bool
	NoElevate bool
	// HTTPEndpoint is the HTTPS endpoint of the deployment, used by --as when
	// the Admin API cannot be reached. The HTTPS transport sets it.
	HTTPEndpoint string

	conn              *grpc.ClientConn
	admin             *AdminClient
	adminHostExplicit bool
	adminTok          string
	asTok             string
	plane             Plane
	need              string
	usedAdmin         bool
}

// Config is the config file, $XDG_CONFIG_HOME/reva/config.toml. The sections
// follow the brand file of the CLI, so that one file format serves both.
type Config struct {
	Connection struct {
		Gateway   string `toml:"gateway,omitempty" json:"gateway,omitempty"`
		AdminHost string `toml:"admin_host,omitempty" json:"admin_host,omitempty"`
	} `toml:"connection" json:"connection"`
	Auth struct {
		Kerberos struct {
			SPN string `toml:"spn,omitempty" json:"spn,omitempty"`
		} `toml:"kerberos,omitempty" json:"kerberos,omitzero"`
	} `toml:"auth,omitempty" json:"auth,omitzero"`
}

// Load fills the settings that are not given as flags from the environment,
// then from the config file.
func (r *Runtime) Load(flags *pflag.FlagSet) error {
	c, err := ReadConfig()
	if err != nil {
		return err
	}
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
	fill(&r.GatewayAddr, "gateway", c.Connection.Gateway, "REVA_GATEWAY", "REVA_HOST")
	fill(&r.AdminHost, "admin-host", c.Connection.AdminHost, "REVA_ADMIN_HOST")
	r.adminHostExplicit = flags.Changed("admin-host") || os.Getenv("REVA_ADMIN_HOST") != ""
	if v, err := strconv.ParseBool(os.Getenv("REVA_NO_ELEVATE")); err == nil && !flags.Changed("no-elevate") {
		r.NoElevate = v
	}
	fill(&r.TokenValue, "token", "", "REVA_TOKEN")
	switch r.Output {
	case "table", "json":
	case "csv":
		return errors.New("csv output is not available yet: use table or json")
	default:
		return fmt.Errorf("unknown output %q: use table or json", r.Output)
	}
	return nil
}

// Client returns a gateway client without credentials, for the calls that
// do not need a login (for example Authenticate).
func (r *Runtime) Client() (gateway.GatewayAPIClient, error) {
	if r.conn == nil {
		if r.GatewayAddr == "" {
			return nil, errors.New("gateway not set: pass --gateway, set REVA_GATEWAY or run `reva config set gateway <host:port>`")
		}
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: r.SkipVerify})
		if r.Insecure {
			creds = insecure.NewCredentials()
		}
		opts := append(r.DialOptions(), grpc.WithTransportCredentials(creds))
		conn, err := grpc.NewClient(r.GatewayAddr, opts...)
		if err != nil {
			return nil, err
		}
		r.conn = conn
	}
	return gateway.NewGatewayAPIClient(r.conn), nil
}

// DialOptions are the options that every gRPC connection of the CLI shares.
func (r *Runtime) DialOptions() []grpc.DialOption {
	if !r.Debug {
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
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: r.SkipVerify}
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
	if r.As == "" {
		return r.LoginToken(ctx)
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

// LoginToken returns the token from --token or REVA_TOKEN. Else it signs in
// with the app token of --app-token-file. Else it returns the token that
// `reva login` stored.
func (r *Runtime) LoginToken(ctx context.Context) (string, error) {
	if r.TokenValue != "" {
		return r.TokenValue, nil
	}
	if r.AppTokenFile != "" {
		b, err := os.ReadFile(r.AppTokenFile)
		if err != nil {
			return "", err
		}
		if r.User == "" {
			return "", errors.New("an app token needs a user name: pass --user")
		}
		t, err := r.Authenticate(ctx, "appauth", r.User, strings.TrimSpace(string(b)))
		if err != nil {
			return "", err
		}
		r.TokenValue = t
		return t, nil
	}
	var b []byte
	var err error
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		b, err = os.ReadFile(p)
	} else {
		b, err = readState("token", ".reva-token")
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("no login token: run `reva login` first")
	}
	return strings.TrimSpace(string(b)), err
}

// TokenPath returns where SetToken stores the login token.
func TokenPath() (string, error) {
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "reva", "token"), err
}

// SetToken stores the login token.
func (r *Runtime) SetToken(t string) error {
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		return os.WriteFile(p, []byte(t), 0o600)
	}
	return writeState("token", []byte(t))
}

// ClearTokens removes the stored login and admin tokens, also the legacy
// dotfiles, so that no older token is read back.
func ClearTokens() error {
	var paths []string
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		paths = append(paths, p)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "reva", "token"), filepath.Join(dir, "reva", "admin-token"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".reva-token"), filepath.Join(home, ".reva-admin-token"))
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ReadConfig returns the stored config. A missing file gives an empty config.
// When the file is missing but the legacy ~/.reva.config exists, ReadConfig
// copies it to the new file, so it reads the legacy file only once.
func ReadConfig() (Config, error) {
	var c Config
	p, err := ConfigPath()
	if err != nil {
		return c, err
	}
	if _, err := toml.DecodeFile(p, &c); err == nil || !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			err = fmt.Errorf("reading %s: %w", p, err)
		}
		return c, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return c, nil
	}
	legacy := filepath.Join(home, ".reva.config")
	b, err := os.ReadFile(legacy)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var old struct {
		Host      string `json:"host"`
		AdminHost string `json:"admin_host"`
	}
	if err := json.Unmarshal(b, &old); err != nil {
		return c, fmt.Errorf("reading %s: %w", legacy, err)
	}
	c.Connection.Gateway, c.Connection.AdminHost = old.Host, old.AdminHost
	if err := WriteConfig(c); err != nil {
		return c, err
	}
	fmt.Fprintf(os.Stderr, "reva: copied the settings of %s to %s\n", legacy, p)
	return c, nil
}

// WriteConfig stores the config.
func WriteConfig(c Config) error {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	return writeState("config.toml", b.Bytes())
}

// ConfigPath returns where WriteConfig stores the config.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "reva", "config.toml"), err
}

// readState reads name from the reva config dir. If that file does not exist,
// it reads the legacy dotfile in the home dir, so old setups keep working.
func readState(name, legacy string) ([]byte, error) {
	if dir, err := os.UserConfigDir(); err == nil {
		b, err := os.ReadFile(filepath.Join(dir, "reva", name))
		if !errors.Is(err, fs.ErrNotExist) {
			return b, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(home, legacy))
}

func writeState(name string, data []byte) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "reva")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o600)
}

// UseColor reports whether to color the output written to f: only for a
// terminal, and never when NO_COLOR is set (https://no-color.org).
func UseColor(f *os.File) bool {
	return os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(f.Fd()))
}

// Msg writes a message for a person to stderr. -q and -o json hide it, so
// that only the result goes to a script.
func (r *Runtime) Msg(w io.Writer, format string, args ...any) {
	if r.Quiet || r.Output == "json" {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}

// Debugf writes a line to stderr with --debug.
func (r *Runtime) Debugf(format string, args ...any) {
	if r.Debug {
		fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
	}
}

// ShowProgress reports whether to draw a progress bar: only for a person
// that watches stderr, and not with -q, --no-progress or -o json.
func (r *Runtime) ShowProgress() bool {
	return !r.Quiet && !r.NoProgress && r.Output != "json" && term.IsTerminal(int(os.Stderr.Fd()))
}

// Print writes v as JSON with -o json. Else it calls text.
func (r *Runtime) Print(w io.Writer, v any, text func(w io.Writer)) error {
	if r.Output != "json" {
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
