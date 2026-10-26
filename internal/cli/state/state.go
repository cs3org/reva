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

// Package state holds the config file and the stored tokens of the reva CLI.
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

	"github.com/BurntSushi/toml"
)

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
	return Write("config.toml", b.Bytes())
}

// ConfigPath returns where WriteConfig stores the config.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "reva", "config.toml"), err
}

// ReadToken returns the stored login token, as SetToken wrote it.
func ReadToken() ([]byte, error) {
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		return os.ReadFile(p)
	}
	return Read("token", ".reva-token")
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
func SetToken(t string) error {
	if p := os.Getenv("REVA_TOKEN_FILE"); p != "" {
		return os.WriteFile(p, []byte(t), 0o600)
	}
	return Write("token", []byte(t))
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

// Read reads name from the reva config dir. If that file does not exist, it
// reads the legacy dotfile in the home dir, so old setups keep working.
func Read(name, legacy string) ([]byte, error) {
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

// Write writes name to the reva config dir, readable only by the user.
func Write(name string, data []byte) error {
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
