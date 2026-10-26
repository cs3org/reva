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
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cs3org/reva/v3"
	"github.com/spf13/cobra"
)

// The namespaces of the CLI plugins in the plugin registry of reva. revad
// skips them.
const (
	CommandsNamespace = "cli.commands"
	AuthNamespace     = "cli.auth"
	PathsNamespace    = "cli.paths"
)

// AuthProvider is a sign-in method of the credential chain.
//
// TODO(T41): this is provisional. The credential chain replaces it.
type AuthProvider interface {
	// Name identifies the provider in `status` and in `login --method`.
	Name() string
	// Available reports whether the provider has what it needs. It must not
	// use the network.
	Available(ctx context.Context) bool
	// Token signs in and returns a credential.
	Token(ctx context.Context) (string, error)
}

// PathResolver turns a path argument into a remote path.
//
// TODO(T41): this is provisional. The path resolver replaces it.
type PathResolver interface {
	// Resolve returns ok false for an argument that is not for this resolver.
	Resolve(ctx context.Context, arg string) (path string, ok bool, err error)
}

// RegisterCommand registers a command plugin. path names the parent and the
// command, for example "otg" (a top-level command) or "user account" (the
// command account under user). Call it in init().
func RegisterCommand(path string, newCmd func(*Runtime) *cobra.Command) {
	register(CommandsNamespace, strings.Fields(path), newCmd)
}

// RegisterAuth registers a sign-in method. Call it in init().
func RegisterAuth(name string, newProvider func(*Runtime) AuthProvider) {
	register(AuthNamespace, []string{name}, newProvider)
}

// RegisterPaths registers a path resolver. Call it in init().
func RegisterPaths(name string, newResolver func(*Runtime) PathResolver) {
	register(PathsNamespace, []string{name}, newResolver)
}

type plugin reva.PluginInfo

func (p plugin) RevaPlugin() reva.PluginInfo { return reva.PluginInfo(p) }

// register panics on a duplicate, because reva.RegisterPlugin replaces a
// plugin with the same id without a sign.
func register(ns string, labels []string, newFn any) {
	if len(labels) == 0 {
		panic("cli: a plugin needs a name in " + ns)
	}
	for _, l := range labels {
		if l == "" || strings.Contains(l, ".") {
			panic(fmt.Sprintf("cli: plugin name %q in %s: a name must not be empty or contain a dot", l, ns))
		}
	}
	id := reva.PluginID(ns + "." + strings.Join(labels, "."))
	if slices.ContainsFunc(reva.GetPlugins(string(id)), func(p reva.PluginInfo) bool { return p.ID == id }) {
		panic("cli: plugin " + string(id) + " is registered twice")
	}
	reva.RegisterPlugin(plugin{ID: id, New: newFn})
}

// PluginConfig decodes the section [plugins.<name>] of the config file into
// v. A missing section leaves v as it is, and an unknown key is an error.
// The settings are loaded when a command runs: in a completion function,
// call Runtime.Load first.
func PluginConfig(rt *Runtime, name string, v any) error {
	sec, ok := rt.config.Plugins[name]
	if !ok {
		return nil
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(sec); err != nil {
		return err
	}
	md, err := toml.Decode(b.String(), v)
	if err != nil {
		return fmt.Errorf("config section [plugins.%s]: %w", name, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fmt.Errorf("config section [plugins.%s]: unknown key %s", name, und[0])
	}
	return nil
}
