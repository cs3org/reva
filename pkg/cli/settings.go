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

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/spf13/pflag"
)

// setting is one setting of the runtime. Its value comes from the first
// layer that has a value that is not empty: the flag, the env names of the
// brand, the user config, the system config, the brand, the built-in
// default.
type setting struct {
	key     string // the key in [env] of the brand
	flag    string
	short   string
	usage   string
	builtin string
	// secret keeps the value out of the flag default that help shows.
	secret bool
	field  func(r *Runtime) any // *string or *bool
	config func(c *state.Config) string
	brand  func(b *brand.Brand) string
}

func boolString(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

var settings = []setting{
	{key: "transport", flag: "transport", usage: "transport of the user plane: grpc, http or auto", builtin: "grpc",
		field:  func(r *Runtime) any { return &r.transport },
		config: func(c *state.Config) string { return c.Connection.Transport },
		brand:  func(b *brand.Brand) string { return b.Connection.Transport }},
	{key: "endpoint", flag: "endpoint", usage: "HTTPS endpoint of the deployment, https://host",
		field:  func(r *Runtime) any { return &r.httpEndpoint },
		config: func(c *state.Config) string { return c.Connection.Endpoint },
		brand:  func(b *brand.Brand) string { return b.Connection.Endpoint }},
	{key: "gateway", flag: "gateway", usage: "address of the gateway, host:port",
		field:  func(r *Runtime) any { return &r.gatewayAddr },
		config: func(c *state.Config) string { return c.Connection.Gateway },
		brand:  func(b *brand.Brand) string { return b.Connection.Gateway }},
	{key: "admin_host", flag: "admin-host", usage: "address of the Admin API, host:port or unix:///path (empty: the gateway)",
		field:  func(r *Runtime) any { return &r.adminHost },
		config: func(c *state.Config) string { return c.Connection.AdminHost },
		brand:  func(b *brand.Brand) string { return b.Connection.AdminHost }},
	{key: "insecure", flag: "insecure", usage: "allow plaintext, without TLS, on the selected transport (development only)",
		field:  func(r *Runtime) any { return &r.insecure },
		config: func(c *state.Config) string { return boolString(c.Connection.Insecure) }},
	{key: "skip_verify", flag: "skip-verify", usage: "do not verify the server certificate (development only)",
		field:  func(r *Runtime) any { return &r.skipVerify },
		config: func(c *state.Config) string { return boolString(c.Connection.SkipVerify) }},
	{key: "token", flag: "token", usage: "use this token instead of signing in", secret: true,
		field: func(r *Runtime) any { return &r.tokenValue }},
	{key: "token_cache", usage: "file of the login token",
		field:  func(r *Runtime) any { return &r.tokenCache },
		config: func(c *state.Config) string { return c.Auth.TokenCache }},
	{key: "app_token", secret: true,
		field: func(r *Runtime) any { return &r.appToken }},
	{key: "app_token_file", flag: "app-token-file", usage: "file holding an app token, to sign in with --user",
		field: func(r *Runtime) any { return &r.appTokenFile }},
	{key: "user", flag: "user", usage: "user name, for app-token and basic sign-in",
		field: func(r *Runtime) any { return &r.user }},
	{key: "password", secret: true,
		field: func(r *Runtime) any { return &r.password }},
	{key: "kerberos_spn",
		field:  func(r *Runtime) any { return &r.kerberosSPN },
		config: func(c *state.Config) string { return c.Auth.Kerberos.SPN },
		brand:  func(b *brand.Brand) string { return b.Auth.Kerberos.SPN }},
	{key: "no_elevate", flag: "no-elevate", usage: "never get an admin token automatically",
		field: func(r *Runtime) any { return &r.noElevate }},
	{key: "output", flag: "output", short: "o", usage: "output format: table or json", builtin: "table",
		field:  func(r *Runtime) any { return &r.output },
		config: func(c *state.Config) string { return c.UI.Output },
		brand:  func(b *brand.Brand) string { return b.UI.Output }},
	{key: "operator_commands", builtin: "show",
		field:  func(r *Runtime) any { return &r.operatorCommands },
		config: func(c *state.Config) string { return c.UI.OperatorCommands },
		brand:  func(b *brand.Brand) string { return b.UI.OperatorCommands }},
}

// value is the resolved value of a setting, and where it comes from.
type value struct {
	v, from string
}

// NewRuntime returns a runtime for the brand b, nil for the reva brand. It
// resolves the settings from the environment, the config files and the
// brand at once, before the command tree is built, so that help and
// completion see them too. The flags override them when they are parsed.
func NewRuntime(b *brand.Brand) *Runtime {
	r := &Runtime{brand: b}
	r.resolve()
	return r
}

// Brand returns the brand of the build.
func (r *Runtime) Brand() *brand.Brand {
	if r.brand == nil {
		return brand.Reva()
	}
	return r.brand
}

// Name returns the name of the program, for the messages to a person.
func (r *Runtime) Name() string { return r.Brand().Name() }

// resolve fills every setting from the layers below the flags. Its errors
// wait for Load, so that help works with a broken config file.
func (r *Runtime) resolve() {
	r.resolved = true
	b := r.Brand()
	var errs []error
	l := state.Layout{Dir: b.ConfigDir()}
	for _, n := range b.Env.Names("config") {
		if v := os.Getenv(n); v != "" {
			l.ConfigFile = v
			break
		}
	}
	state.Use(l)
	sys, err := state.ReadSystemConfig()
	if err != nil {
		errs = append(errs, err)
	}
	user, from, err := state.PeekConfig()
	if err != nil {
		errs = append(errs, err)
	}
	r.legacyConfig = from
	userPath, _ := state.ConfigPath()

	r.values = map[string]value{}
	for _, s := range settings {
		v := r.lookup(s, b, []layer{{"user config " + userPath, &user}, {"system config " + state.SystemConfigPath(), &sys}})
		r.values[s.key] = v
		if err := s.set(r, v); err != nil {
			errs = append(errs, err)
		}
	}
	if r.tokenCache != "" {
		l.TokenFile = r.tokenCache
		state.Use(l)
	}

	r.config = user
	r.config.Plugins = maps.Clone(sys.Plugins)
	for name, sec := range user.Plugins {
		merged := maps.Clone(r.config.Plugins[name])
		if merged == nil {
			merged = map[string]any{}
		}
		maps.Copy(merged, sec)
		if r.config.Plugins == nil {
			r.config.Plugins = map[string]map[string]any{}
		}
		r.config.Plugins[name] = merged
	}
	r.resolveErr = errors.Join(errs...)
	warnForeignEnv(b)
}

type layer struct {
	name string
	c    *state.Config
}

func (r *Runtime) lookup(s setting, b *brand.Brand, files []layer) value {
	for _, n := range b.Env.Names(s.key) {
		if v := os.Getenv(n); v != "" {
			return value{v, "env " + n}
		}
	}
	if s.config != nil {
		for _, f := range files {
			if v := s.config(f.c); v != "" {
				return value{v, f.name}
			}
		}
	}
	if s.brand != nil {
		if v := s.brand(b); v != "" {
			return value{v, "brand " + b.Name()}
		}
	}
	return value{s.builtin, "built-in"}
}

func (s setting) set(r *Runtime, v value) error {
	switch p := s.field(r).(type) {
	case *string:
		*p = v.v
	case *bool:
		if v.v == "" {
			*p = false
			return nil
		}
		b, err := strconv.ParseBool(v.v)
		if err != nil {
			return fmt.Errorf("%s: %q is not true or false", v.from, v.v)
		}
		*p = b
	}
	return nil
}

func (s setting) get(r *Runtime) string {
	switch p := s.field(r).(type) {
	case *string:
		return *p
	case *bool:
		return strconv.FormatBool(*p)
	}
	return ""
}

// addSettingFlags adds the flags of the settings. The default of a flag is
// the resolved value, so that help shows it.
func (r *Runtime) addSettingFlags(fs *pflag.FlagSet) {
	if !r.resolved {
		r.resolve()
	}
	for _, s := range settings {
		if s.flag == "" {
			continue
		}
		usage := s.usage + r.envHint(s.key)
		switch p := s.field(r).(type) {
		case *string:
			v, def := *p, *p
			if s.secret {
				def = ""
			}
			fs.StringVarP(p, s.flag, s.short, def, usage)
			*p = v
		case *bool:
			fs.BoolVarP(p, s.flag, s.short, *p, usage)
		}
	}
}

func (r *Runtime) envHint(key string) string {
	if names := r.Brand().Env.Names(key); len(names) > 0 {
		return " (env " + strings.Join(names, ", ") + ")"
	}
	return ""
}

// Load runs when the flags are parsed. It reports the errors of the
// settings, and records which ones the flags set. A command runs it in the
// hook of the root; a completion function calls it itself.
func (r *Runtime) Load(flags *pflag.FlagSet) error {
	if !r.resolved {
		r.resolve()
	}
	for _, s := range settings {
		if s.flag != "" && flags.Changed(s.flag) {
			r.values[s.key] = value{s.get(r), "flag --" + s.flag}
		}
	}
	from := r.values["admin_host"].from
	r.adminHostExplicit = strings.HasPrefix(from, "flag") || strings.HasPrefix(from, "env")
	if r.resolveErr != nil {
		return r.resolveErr
	}
	if r.legacyConfig != "" {
		r.legacyConfig = ""
		if _, err := state.ReadConfig(); err != nil {
			return err
		}
	}
	if !slices.Contains(brand.Transports, r.transport) {
		return fmt.Errorf("unknown transport %q (%s): use %s", r.transport, r.values["transport"].from, strings.Join(brand.Transports, ", "))
	}
	if !slices.Contains(brand.OperatorModes, r.operatorCommands) {
		return fmt.Errorf("unknown operator_commands %q (%s): use %s", r.operatorCommands, r.values["operator_commands"].from, strings.Join(brand.OperatorModes, ", "))
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

// Setting returns the value of a setting, a key of [env] of the brand, and
// where it comes from: a flag, an env name, a config file, the brand or
// built-in.
func (r *Runtime) Setting(key string) (v, from string) {
	if !r.resolved {
		r.resolve()
	}
	s := r.values[key]
	return s.v, s.from
}

// stderr is where the warnings go. Tests replace it.
var stderr io.Writer = os.Stderr

var foreignEnvOnce sync.Once

// warnForeignEnv warns once when a build that is not reva sees a REVA_*
// variable, so that a script written for reva does not reach the wrong
// deployment without a sign.
func warnForeignEnv(b *brand.Brand) {
	if b.Name() == "reva" {
		return
	}
	own := map[string]bool{}
	for _, key := range brand.EnvKeys() {
		for _, n := range b.Env.Names(key) {
			own[n] = true
		}
	}
	var found []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "REVA_") && !own[k] {
			found = append(found, k)
		}
	}
	if len(found) == 0 {
		return
	}
	slices.Sort(found)
	foreignEnvOnce.Do(func() {
		fmt.Fprintf(stderr, "%s: ignoring %s: %s does not read the variables of reva\n", b.Name(), strings.Join(found, ", "), b.Name())
	})
}

// notSet is the error for a missing address, with the ways to set it.
func (r *Runtime) notSet(what, key, flag, example string) error {
	hint := "pass " + flag
	if names := r.Brand().Env.Names(key); len(names) > 0 {
		hint += ", set " + names[0]
	}
	return fmt.Errorf("%s not set: %s or run `%s config set %s`", what, hint, r.Name(), example)
}

// defaultAuthTypes are the auth types of a stock reva gateway.
var defaultAuthTypes = map[string]string{"app-token": "appauth", "basic": "basic", "kerberos": "kerberos", "bearer": "bearer"}

// AuthType returns the auth type of the gateway for a kind of credential
// (app-token, basic, kerberos, bearer): from auth.types of the brand, else
// the name of a stock reva gateway.
func (r *Runtime) AuthType(kind string) string {
	if t := r.Brand().Auth.Types[kind]; t != "" {
		return t
	}
	if t, ok := defaultAuthTypes[kind]; ok {
		return t
	}
	return kind
}

// ShowOperators reports whether help shows the operator commands and
// options, from ui.operator_commands: always with show, never with hide.
// With auto, only for an operator: when the local admin socket answers, an
// admin host is configured, or an admin token of elevate exists. It makes no
// network call, and hidden commands still run.
func (r *Runtime) ShowOperators() bool {
	switch r.operatorCommands {
	case "hide":
		return false
	case "auto":
		return r.adminHost != "" || socketAnswers() || hasAdminToken()
	}
	return true
}

func socketAnswers() bool {
	for _, p := range socketPaths() {
		if fi, err := os.Stat(p); err != nil || fi.Mode()&os.ModeSocket == 0 {
			continue
		}
		if c, err := net.DialTimeout("unix", p, 200*time.Millisecond); err == nil {
			_ = c.Close()
			return true
		}
	}
	return false
}

func hasAdminToken() bool {
	b, err := state.Read("admin-token", ".reva-admin-token")
	if err != nil {
		return false
	}
	var s storedAdmin
	return json.Unmarshal(b, &s) == nil && s.Token != "" && usable(s.Token)
}
