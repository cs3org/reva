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

// Package state holds the config files and the stored tokens of the reva CLI.
// It is internal so that CLI plugins do not depend on the file layout.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is a config file: the user config, $XDG_CONFIG_HOME/<dir>/config.toml,
// or the system config, /etc/<dir>/config.toml. The sections follow the
// brand file of the CLI, so that one file format serves both.
type Config struct {
	Connection struct {
		Transport  string `toml:"transport,omitempty" json:"transport,omitempty"`
		Endpoint   string `toml:"endpoint,omitempty" json:"endpoint,omitempty"`
		Gateway    string `toml:"gateway,omitempty" json:"gateway,omitempty"`
		AdminHost  string `toml:"admin_host,omitempty" json:"admin_host,omitempty"`
		Insecure   *bool  `toml:"insecure,omitempty" json:"insecure,omitempty"`
		SkipVerify *bool  `toml:"skip_verify,omitempty" json:"skip_verify,omitempty"`
	} `toml:"connection" json:"connection"`
	Auth struct {
		TokenCache string `toml:"token_cache,omitempty" json:"token_cache,omitempty"`
		Kerberos   struct {
			SPN string `toml:"spn,omitempty" json:"spn,omitempty"`
		} `toml:"kerberos,omitempty" json:"kerberos,omitzero"`
	} `toml:"auth,omitempty" json:"auth,omitzero"`
	Transfer struct {
		ChunkSize string `toml:"chunk_size,omitempty" json:"chunk_size,omitempty"`
		Archive   *bool  `toml:"archive,omitempty" json:"archive,omitempty"`
		Jobs      int    `toml:"jobs,omitempty" json:"jobs,omitempty"`
	} `toml:"transfer,omitempty" json:"transfer,omitzero"`
	UI struct {
		Output           string `toml:"output,omitempty" json:"output,omitempty"`
		OperatorCommands string `toml:"operator_commands,omitempty" json:"operator_commands,omitempty"`
	} `toml:"ui,omitempty" json:"ui,omitzero"`
	// Plugins are the [plugins.<name>] sections. They are kept as they are,
	// so that WriteConfig does not drop them, and each plugin decodes its own.
	Plugins map[string]map[string]any `toml:"plugins,omitempty" json:"plugins,omitempty"`
}

// Layout says where the files are. The brand of the build sets it once,
// before the first read.
type Layout struct {
	// Dir is the name of the config dir, the config_dir of the brand.
	Dir string
	// ConfigFile replaces the user config file when it is set.
	ConfigFile string
	// TokenFile replaces the file of the login token when it is set.
	TokenFile string
}

var layout = Layout{Dir: "reva"}

// Use sets the layout of the files.
func Use(l Layout) { layout = l }

// SystemDir is the root of the system config. Tests replace it.
var SystemDir = func() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("ProgramData")
	}
	return "/etc"
}

// legacy reports whether to read the dotfiles of the older reva CLI. Only
// the reva build had them.
func legacy() bool { return layout.Dir == "reva" }

// ReadConfig returns the user config. A missing file gives an empty config.
// When the file is missing but a legacy one exists, ReadConfig copies it to
// the new file, so it reads the legacy file only once.
func ReadConfig() (Config, error) {
	c, from, dropped, err := peekConfig()
	if err != nil || from == "" {
		return c, err
	}
	if err := WriteConfig(c); err != nil {
		return c, err
	}
	p, _ := ConfigPath()
	fmt.Fprintf(os.Stderr, "%s: copied the settings of %s to %s\n", layout.Dir, from, p)
	if len(dropped) > 0 {
		fmt.Fprintf(os.Stderr, "%s: not copied, because config.toml has no place for them: %s\n", layout.Dir, strings.Join(dropped, ", "))
	}
	return c, nil
}

// PeekConfig is ReadConfig without the copy: from names the legacy file
// that the config comes from, if any.
func PeekConfig() (c Config, from string, err error) {
	c, from, _, err = peekConfig()
	return c, from, err
}

func peekConfig() (c Config, from string, dropped []string, err error) {
	p, err := ConfigPath()
	if err != nil {
		return c, "", nil, err
	}
	if c, err := decodeFile(p); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return c, "", nil, err
	}
	if layout.ConfigFile != "" {
		return c, "", nil, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return c, "", nil, nil
	}
	yml := filepath.Join(dir, layout.Dir, "config.yaml")
	if c, dropped, err := readYAML(yml); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return c, yml, dropped, err
	}
	home, err := os.UserHomeDir()
	if err != nil || !legacy() {
		return c, "", nil, nil
	}
	old := filepath.Join(home, ".reva.config")
	b, err := os.ReadFile(old)
	if errors.Is(err, fs.ErrNotExist) {
		return c, "", nil, nil
	}
	if err != nil {
		return c, "", nil, err
	}
	var v struct {
		Host      string `json:"host"`
		AdminHost string `json:"admin_host"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return c, "", nil, fmt.Errorf("reading %s: %w", old, err)
	}
	c.Connection.Gateway, c.Connection.AdminHost = v.Host, v.AdminHost
	return c, old, nil, nil
}

// ReadSystemConfig returns the system config, which an operator (puppet)
// writes. A missing file gives an empty config.
func ReadSystemConfig() (Config, error) {
	c, err := decodeFile(SystemConfigPath())
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	return c, err
}

// decodeFile is strict: an unknown key outside [plugins] is an error, so
// that a typo does not go unseen.
func decodeFile(p string) (Config, error) {
	var c Config
	md, err := toml.DecodeFile(p, &c)
	if err == nil {
		// The keys of [plugins] are checked by each plugin's PluginConfig, but
		// the decoder still reports the keys of an array of tables in them.
		for _, k := range md.Undecoded() {
			if len(k) > 0 && k[0] == "plugins" {
				continue
			}
			err = fmt.Errorf("unknown key %s", k)
			break
		}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		err = fmt.Errorf("reading %s: %w", p, err)
	}
	return c, err
}

// WriteConfig stores the user config. It writes only TOML.
func WriteConfig(c Config) error {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	p, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, b.Bytes(), 0o600)
}

// ConfigPath returns the user config file.
func ConfigPath() (string, error) {
	if layout.ConfigFile != "" {
		return layout.ConfigFile, nil
	}
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, layout.Dir, "config.toml"), err
}

// SystemConfigPath returns the system config file.
func SystemConfigPath() string {
	return filepath.Join(SystemDir(), layout.Dir, "config.toml")
}

// ReadToken returns the stored login token, as SetToken wrote it.
func ReadToken() ([]byte, error) {
	if layout.TokenFile != "" {
		return os.ReadFile(layout.TokenFile)
	}
	return Read("token", ".reva-token")
}

// TokenPath returns where SetToken stores the login token.
func TokenPath() (string, error) {
	if layout.TokenFile != "" {
		return layout.TokenFile, nil
	}
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, layout.Dir, "token"), err
}

// SetToken stores the login token.
func SetToken(t string) error {
	if layout.TokenFile != "" {
		return os.WriteFile(layout.TokenFile, []byte(t), 0o600)
	}
	return Write("token", []byte(t))
}

// ClearTokens removes the stored login and admin tokens, also the legacy
// dotfiles, so that no older token is read back.
func ClearTokens() error {
	var paths []string
	if layout.TokenFile != "" {
		paths = append(paths, layout.TokenFile)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, layout.Dir, "token"), filepath.Join(dir, layout.Dir, "admin-token"))
	}
	if home, err := os.UserHomeDir(); err == nil && legacy() {
		paths = append(paths, filepath.Join(home, ".reva-token"), filepath.Join(home, ".reva-admin-token"))
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Read reads name from the config dir. If that file does not exist, it
// reads the legacy dotfile in the home dir, so old setups keep working.
func Read(name, legacyName string) ([]byte, error) {
	dir, err := os.UserConfigDir()
	if err == nil {
		b, err := os.ReadFile(filepath.Join(dir, layout.Dir, name))
		if !errors.Is(err, fs.ErrNotExist) || !legacy() {
			return b, err
		}
	} else if !legacy() {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(home, legacyName))
}

// Write writes name to the config dir, readable only by the user.
func Write(name string, data []byte) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, layout.Dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o600)
}
