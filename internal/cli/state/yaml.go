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

package state

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// readYAML reads the config.yaml of cernbox-cli. dropped are the keys that
// Config has no place for.
func readYAML(p string) (c Config, dropped []string, err error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return c, nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return c, nil, fmt.Errorf("reading %s: %w", p, err)
	}
	// take removes the value at path from m, so that what is left over is
	// what is dropped.
	take := func(path string) any {
		keys := strings.Split(path, ".")
		cur := m
		for _, k := range keys[:len(keys)-1] {
			next, ok := cur[k].(map[string]any)
			if !ok {
				return nil
			}
			cur = next
		}
		v := cur[keys[len(keys)-1]]
		delete(cur, keys[len(keys)-1])
		return v
	}
	str := func(path string) string {
		s, _ := take(path).(string)
		return s
	}
	boolp := func(path string) *bool {
		if v, ok := take(path).(bool); ok {
			return &v
		}
		return nil
	}
	c.Connection.Endpoint = str("endpoint")
	c.Connection.Insecure = boolp("insecure")
	c.Auth.TokenCache = str("auth.token_cache")
	c.Auth.Kerberos.SPN = str("auth.kerberos.service_principal")
	c.Transfer.ChunkSize = str("transfer.chunk_size")
	c.Transfer.Archive = boolp("transfer.archive")
	c.Transfer.Jobs, _ = take("transfer.jobs").(int)
	// The settings of edit, inbox and outbox move to the sections of their
	// commands, which plugins decode.
	plugin := func(name, key string, v any) {
		if v == nil {
			return
		}
		if c.Plugins == nil {
			c.Plugins = map[string]map[string]any{}
		}
		if key == "" {
			if sec, ok := v.(map[string]any); ok {
				c.Plugins[name] = sec
			}
			return
		}
		c.Plugins[name] = map[string]any{key: v}
	}
	plugin("edit", "", take("edit"))
	plugin("inbox", "folders", take("inbox"))
	plugin("outbox", "folders", take("outbox"))
	return c, leftover("", m), nil
}

func leftover(prefix string, m map[string]any) []string {
	var keys []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if sub, ok := m[k].(map[string]any); ok {
			keys = append(keys, leftover(prefix+k+".", sub)...)
		} else if m[k] != nil {
			keys = append(keys, prefix+k)
		}
	}
	return keys
}
