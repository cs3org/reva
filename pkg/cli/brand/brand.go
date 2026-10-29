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

// Package brand holds the brand of a build of the CLI: the program name, the
// defaults, the env names and the texts. A brand is a TOML file that a build
// embeds (gaia --brand). The struct tags are its only schema.
package brand

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Brand is a parsed brand file. Its values are defaults: flags, env and config
// files override them. Do not change a Brand after Parse.
type Brand struct {
	Program    Program    `toml:"program"`
	Connection Connection `toml:"connection"`
	Auth       Auth       `toml:"auth"`
	Env        Env        `toml:"env"`
	Transfer   Transfer   `toml:"transfer"`
	Messages   Messages   `toml:"messages"`
	UI         UI         `toml:"ui"`

	sum string
}

// Program names the program.
type Program struct {
	Name       string `toml:"name"`
	Short      string `toml:"short"`
	SupportURL string `toml:"support_url"`
	DocsURL    string `toml:"docs_url"`
	// ConfigDir is the directory of the config files, under the user config
	// dir and under /etc. Empty means Name.
	ConfigDir string `toml:"config_dir"`
}

// Connection holds the default addresses.
type Connection struct {
	Transport string `toml:"transport"`
	// Endpoint is the HTTPS user plane.
	Endpoint string `toml:"endpoint"`
	// Gateway is the gRPC user plane.
	Gateway string `toml:"gateway"`
	// AdminHost is the Admin API. The local admin socket comes first.
	AdminHost string `toml:"admin_host"`
}

// Auth configures the sign-in.
type Auth struct {
	// Chain is the order of the sign-in methods: built-in ones or cli.auth.*
	// plugins.
	Chain    []string `toml:"chain"`
	Kerberos Kerberos `toml:"kerberos"`
	OIDC     OIDC     `toml:"oidc"`
	// Types maps a kind of credential to the auth type of the gateway, for a
	// sign-in over gRPC Authenticate. The deployment decides these names.
	Types map[string]string `toml:"types"`
}

// Kerberos configures the Kerberos sign-in.
type Kerberos struct {
	Mode string `toml:"mode"`
	// Path is the HTTP path that accepts a SPNEGO token.
	Path string `toml:"path"`
	// SPN is the service principal. Empty means HTTP/<host of the server>.
	SPN string `toml:"spn"`
}

// OIDC configures the OIDC device flow.
type OIDC struct {
	Issuer   string   `toml:"issuer"`
	ClientID string   `toml:"client_id"`
	Scopes   []string `toml:"scopes"`
}

// Env lists the env names of each setting, first one first. There is no prefix
// rule, so that the names of older programs keep working.
type Env struct {
	Config           []string `toml:"config"`
	Transport        []string `toml:"transport"`
	Endpoint         []string `toml:"endpoint"`
	Gateway          []string `toml:"gateway"`
	AdminHost        []string `toml:"admin_host"`
	Insecure         []string `toml:"insecure"`
	SkipVerify       []string `toml:"skip_verify"`
	Token            []string `toml:"token"`
	TokenCache       []string `toml:"token_cache"`
	AppToken         []string `toml:"app_token"`
	AppTokenFile     []string `toml:"app_token_file"`
	User             []string `toml:"user"`
	Password         []string `toml:"password"`
	KerberosSPN      []string `toml:"kerberos_spn"`
	NoElevate        []string `toml:"no_elevate"`
	Output           []string `toml:"output"`
	OperatorCommands []string `toml:"operator_commands"`
}

// Names returns the env names of the setting key.
func (e Env) Names(key string) []string {
	for f, v := range reflect.ValueOf(e).Fields() {
		if f.Tag.Get("toml") == key {
			return v.Interface().([]string)
		}
	}
	return nil
}

// EnvKeys returns the keys of [env].
func EnvKeys() []string {
	var keys []string
	for f := range reflect.TypeFor[Env]().Fields() {
		keys = append(keys, f.Tag.Get("toml"))
	}
	return keys
}

// Transfer holds the defaults of the transfers.
type Transfer struct {
	// ChunkSize is a size with a K, M or G suffix, for example 8M.
	ChunkSize string `toml:"chunk_size"`
	// Archive lets a recursive download use the archiver.
	Archive *bool `toml:"archive"`
}

// Messages are the remedy texts of doctor and of some errors.
type Messages struct {
	NetworkHint string `toml:"network_hint"`
	NoHomeHint  string `toml:"no_home_hint"`
}

// UI configures help and output.
type UI struct {
	Output string `toml:"output"`
	// Hide are help groups kept out of help. Their commands still run.
	Hide []string `toml:"hide"`
	// OperatorCommands is auto, show or hide.
	OperatorCommands string `toml:"operator_commands"`
}

// The values that a brand, a config file or a flag can take.
var (
	Transports    = []string{"grpc", "http", "auto"}
	Outputs       = []string{"table", "json", "csv"}
	OperatorModes = []string{"auto", "show", "hide"}
)

var (
	nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sizeRE = regexp.MustCompile(`^[0-9]+[kKmMgG]?$`)
)

// Parse parses a brand file. It is strict, so that a typo fails the build and
// not the users.
func Parse(data []byte) (*Brand, error) {
	var b Brand
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&b)
	if err != nil {
		return nil, fmt.Errorf("brand: %w", err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("brand: unknown key %s", strings.Join(keys, ", "))
	}
	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("brand: %w", err)
	}
	sum := sha256.Sum256(data)
	b.sum = hex.EncodeToString(sum[:])
	return &b, nil
}

// MustParse is Parse for a brand that the build embeds. It panics on error.
func MustParse(data []byte) *Brand {
	b, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return b
}

func (b *Brand) validate() error {
	var errs []error
	if !nameRE.MatchString(b.Program.Name) {
		errs = append(errs, fmt.Errorf("program.name %q: use lower case letters, digits and dashes", b.Program.Name))
	}
	if d := b.Program.ConfigDir; d != "" && (strings.ContainsAny(d, `/\`) || d == "." || d == "..") {
		errs = append(errs, fmt.Errorf("program.config_dir %q: use a directory name, not a path", d))
	}
	oneOf := func(key, v string, valid []string) {
		if v != "" && !slices.Contains(valid, v) {
			errs = append(errs, fmt.Errorf("%s %q: use one of %s", key, v, strings.Join(valid, ", ")))
		}
	}
	oneOf("connection.transport", b.Connection.Transport, Transports)
	oneOf("ui.output", b.UI.Output, Outputs)
	oneOf("ui.operator_commands", b.UI.OperatorCommands, OperatorModes)
	if s := b.Transfer.ChunkSize; s != "" && !sizeRE.MatchString(s) {
		errs = append(errs, fmt.Errorf("transfer.chunk_size %q: use a number with an optional K, M or G", s))
	}
	for _, p := range b.Auth.Chain {
		if p == "" {
			errs = append(errs, errors.New("auth.chain: an empty provider name"))
		}
	}
	for _, g := range b.UI.Hide {
		if g == "" {
			errs = append(errs, errors.New("ui.hide: an empty group"))
		}
	}
	// One env name for two settings would make one of them depend on the
	// order of the checks.
	seen := map[string]string{}
	for _, key := range EnvKeys() {
		for _, n := range b.Env.Names(key) {
			switch {
			case !envRE.MatchString(n):
				errs = append(errs, fmt.Errorf("env.%s: %q is not an env name", key, n))
			case seen[n] != "":
				errs = append(errs, fmt.Errorf("env.%s: %s is also in env.%s", key, n, seen[n]))
			default:
				seen[n] = key
			}
		}
	}
	return errors.Join(errs...)
}

// Name returns the program name.
func (b *Brand) Name() string { return b.Program.Name }

// ConfigDir returns the directory name of the config files.
func (b *Brand) ConfigDir() string {
	if b.Program.ConfigDir != "" {
		return b.Program.ConfigDir
	}
	return b.Program.Name
}

// Sum returns the SHA-256 of the brand file.
func (b *Brand) Sum() string { return b.sum }

//go:embed reva.toml
var revaFile []byte

var reva = sync.OnceValue(func() *Brand { return MustParse(revaFile) })

// Reva returns the brand of the stock reva CLI.
func Reva() *Brand { return reva() }
