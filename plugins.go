// Copyright 2018-2023 CERN
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

package reva

import (
	"sort"
	"strings"

	"github.com/cs3org/reva/pkg/plugin"
)

// Plugin is a type used as reva plugin.
// The type may implement something useful, depending
// on what is possible to plug in.
type Plugin interface {
	// RevaPlugin returns the plugin info, like the ID
	// in the form of <namespace>.<name>, and a New func
	// used to create the plugin.
	// The namespace can be only one defined by reva,
	// depending on the scope of the plugin, while the name
	// can be whatever, but unique in the namespace.
	RevaPlugin() PluginInfo
}

// PluginInfo holds the information of a reva plugin.
type PluginInfo struct {
	// ID is the full name of the plugin, in the form
	// <namespace>.<name>. It must be unique.
	ID PluginID
	// New is the constructor of the plugin. We rely
	// on the developer to correctly provide a valid
	// construct depending on the plugin.
	New any
}

// String return a string representation of the PluginInfo.
func (pi PluginInfo) String() string { return string(pi.ID) }

// PluginID is the string that uniquely identify a reva plugin.
// It consists of a dot-separated labels. The last label is the
// name of the plugin, while the labels before represents the
// namespace.
// A pluginID is in the form <namespace>.<name>
// Neither the name nor the namespace can be empty.
// The name cannot contain dots.
// ModuleIDs shuld be lowercase and use underscores (_) instead
// of spaces.
type PluginID string

var registry = map[string]Plugin{}

// Name returns the name of the Plugin ID (i.e. the last name).
func (i PluginID) Name() string {
	parts := strings.Split(string(i), ".")
	if len(parts) <= 1 {
		panic("plugin id must be <namespace>.<name>")
	}
	return parts[len(parts)-1]
}

// Namespace returns the namespace of the plugin ID, which is
// all but the last label of the ID.
func (i PluginID) Namespace() string {
	idx := strings.LastIndex(string(i), ".")
	if idx < 0 {
		panic("plugin id must be <namespace>.<name>")
	}
	return string(i)[:idx]
}

// RegisterPlugin registers a reva plugin. For registration
// this method should be called in the init() method.
func RegisterPlugin(p Plugin) {
	if p == nil {
		panic("plugin cannot be nil")
	}
	plug := p.RevaPlugin()
	if plug.ID == "" {
		panic("plugin id cannot be nil")
	}
	if plug.New == nil {
		panic("plugin new func cannot be nil")
	}

	name := plug.ID.Name()
	ns := plug.ID.Namespace()
	registry[string(p.RevaPlugin().ID)] = p
	plugin.RegisterPlugin(ns, name, plug.New)
}

func hasPrefixSlices(s, prefix []string) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := range prefix {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

// GetPlugins returns all the plugins in the given namespace,
// and their descendants.
// For example, a namespace "foo" returns modules with id "foo",
// "foo.bar", "foo.bar.foo", but not "bar".
func GetPlugins(ns string) []PluginInfo {
	prefix := strings.Split(ns, ".")
	if ns == "" {
		prefix = []string{}
	}

	var plugs []PluginInfo
	for ns, p := range registry {
		nsParts := strings.Split(ns, ".")
		if hasPrefixSlices(nsParts, prefix) {
			plugs = append(plugs, p.RevaPlugin())
		}
	}

	sort.SliceStable(plugs, func(i, j int) bool {
		return plugs[i].ID < plugs[j].ID
	})
	return plugs
}
