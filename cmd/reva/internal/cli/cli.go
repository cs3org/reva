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
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Runtime is built once per process from the flags, the environment and the
// config file, in this order of precedence.
type Runtime struct {
	Host       string
	AdminHost  string
	Insecure   bool
	SkipVerify bool
	TokenFile  string
	Output     string
	Timeout    time.Duration

	conn *grpc.ClientConn
}

// Config is the part of the settings that `reva config set` stores.
type Config struct {
	Host      string `json:"host,omitempty"`
	AdminHost string `json:"admin_host,omitempty"`
}

// Load fills the settings that are not given as flags from the environment,
// then from the config file.
func (r *Runtime) Load(flags *pflag.FlagSet) error {
	c, err := ReadConfig()
	if err != nil {
		return err
	}
	fill := func(dst *string, flag, env, conf string) {
		if flags.Changed(flag) {
			return
		}
		if v := os.Getenv(env); v != "" {
			*dst = v
			return
		}
		*dst = conf
	}
	fill(&r.Host, "host", "REVA_HOST", c.Host)
	fill(&r.AdminHost, "admin-host", "REVA_ADMIN_HOST", c.AdminHost)
	fill(&r.TokenFile, "token-file", "REVA_TOKEN_FILE", "")
	if r.Output != "text" && r.Output != "json" {
		return fmt.Errorf("unknown output %q: use text or json", r.Output)
	}
	return nil
}

// Client returns a gateway client without credentials, for the calls that
// do not need a login (for example Authenticate).
func (r *Runtime) Client() (gateway.GatewayAPIClient, error) {
	if r.conn == nil {
		if r.Host == "" {
			return nil, errors.New("gateway host not set: pass --host, set REVA_HOST or run `reva config set host <host:port>`")
		}
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: r.SkipVerify})
		if r.Insecure {
			creds = insecure.NewCredentials()
		}
		conn, err := grpc.NewClient(r.Host, grpc.WithTransportCredentials(creds))
		if err != nil {
			return nil, err
		}
		r.conn = conn
	}
	return gateway.NewGatewayAPIClient(r.conn), nil
}

// Gateway returns a gateway client and ctx with the login token attached.
func (r *Runtime) Gateway(ctx context.Context) (gateway.GatewayAPIClient, context.Context, error) {
	client, err := r.Client()
	if err != nil {
		return nil, nil, err
	}
	t, err := r.Token()
	if err != nil {
		return nil, nil, err
	}
	return client, WithToken(ctx, t), nil
}

// WithToken attaches a reva token to ctx and to the outgoing gRPC metadata.
func WithToken(ctx context.Context, t string) context.Context {
	ctx = appctx.ContextSetToken(ctx, t)
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, t)
}

// HTTPClient returns the client for the data transfers. It sends the token
// in the request context. The command context sets the deadline, so the
// client has no timeout of its own.
func (r *Runtime) HTTPClient() *httpclient.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: r.SkipVerify}
	return httpclient.New(httpclient.RoundTripper(tr))
}

// Close releases the gateway connection.
func (r *Runtime) Close() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

// Token returns REVA_TOKEN, or else the token that `reva login` stored.
func (r *Runtime) Token() (string, error) {
	if t := os.Getenv("REVA_TOKEN"); t != "" {
		return t, nil
	}
	var b []byte
	var err error
	if r.TokenFile != "" {
		b, err = os.ReadFile(r.TokenFile)
	} else {
		b, err = readState("token", ".reva-token")
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("no login token: run `reva login` first")
	}
	return strings.TrimSpace(string(b)), err
}

// SetToken stores the login token.
func (r *Runtime) SetToken(t string) error {
	if r.TokenFile != "" {
		return os.WriteFile(r.TokenFile, []byte(t), 0o600)
	}
	return writeState("token", []byte(t))
}

// AdminToken returns the short-TTL token that `reva admin elevate` stored.
func (r *Runtime) AdminToken() (string, error) {
	b, err := readState("admin-token", ".reva-admin-token")
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(b) == 0) {
		return "", errors.New("no admin token: run `reva admin elevate` first")
	}
	return strings.TrimSpace(string(b)), err
}

// SetAdminToken stores the admin token.
func (r *Runtime) SetAdminToken(t string) error {
	return writeState("admin-token", []byte(t))
}

// ReadConfig returns the stored config. A missing file gives an empty config.
func ReadConfig() (Config, error) {
	var c Config
	b, err := readState("config.json", ".reva.config")
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("reading config: %w", err)
	}
	return c, nil
}

// WriteConfig stores the config.
func WriteConfig(c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeState("config.json", append(b, '\n'))
}

// ConfigPath returns where WriteConfig stores the config.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "reva", "config.json"), err
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

// Print writes v as JSON with --output json. Else it calls text.
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
