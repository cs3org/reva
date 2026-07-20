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

package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminConfigCommand() *command {
	cmd := newCommand("config")
	cmd.Description = func() string { return "show a service's effective (redacted) config (TOML by default)" }
	cmd.Usage = func() string {
		return "Usage: admin config [-admin-host h] [-o toml|json] [-diff] <service|node-id>"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	output := cmd.String("o", "toml", "output format: toml | json")
	diff := cmd.Bool("diff", false, "show only the config keys that differ across a service's instances")
	cmd.ResetFlags = func() { *adminHost, *output, *diff = "", "toml", false }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		res, err := client.GetServiceConfig(ctx, &adminpb.GetServiceConfigRequest{Service: cmd.Args()[0]})
		if err != nil {
			return adminErr(err)
		}
		if len(res.Results) == 0 {
			fmt.Println("(no instances)")
			return nil
		}
		if *diff {
			return diffConfigs(res.Results)
		}
		// Header each instance only when there is more than one.
		for i, r := range res.Results {
			if len(res.Results) > 1 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("# %s\n", r.Node)
			}
			if r.Error != "" {
				fmt.Printf("# error: %s\n", r.Error)
				continue
			}
			if err := renderConfig(r.ResultJson, *output); err != nil {
				return err
			}
		}
		return nil
	}
	return cmd
}

// diffConfigs reports the config keys that differ across a service's instances:
// for each flattened key whose value is not unanimous, it groups the distinct
// values by the nodes that hold them, so the odd instance stands out. An absent
// key is shown as "(absent)".
func diffConfigs(results []*adminpb.NodeResult) error {
	type inst struct {
		node string
		flat map[string]string
	}
	var insts []inst
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("# %s: error: %s\n", r.Node, r.Error)
			continue
		}
		var m map[string]any
		dec := json.NewDecoder(strings.NewReader(r.ResultJson))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			fmt.Printf("# %s: unparseable config\n", r.Node)
			continue
		}
		flat := map[string]string{}
		flattenConfig("", m, flat)
		insts = append(insts, inst{node: r.Node, flat: flat})
	}
	if len(insts) < 2 {
		fmt.Println("# need at least two instances to diff (a service name selects them all)")
		return nil
	}

	// Union of all keys.
	keys := map[string]bool{}
	for _, in := range insts {
		for k := range in.flat {
			keys[k] = true
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	// address and network are per-instance by nature (each binds its own), so
	// they always differ and only add noise to a drift comparison.
	perInstance := map[string]bool{"address": true, "network": true}

	const absent = "(absent)"
	differing := 0
	for _, k := range sorted {
		if perInstance[k] {
			continue
		}
		byValue := map[string][]string{}
		for _, in := range insts {
			v, ok := in.flat[k]
			if !ok {
				v = absent
			}
			byValue[v] = append(byValue[v], in.node)
		}
		if len(byValue) == 1 {
			continue // unanimous
		}
		differing++
		fmt.Printf("%s:\n", k)
		vals := make([]string, 0, len(byValue))
		for v := range byValue {
			vals = append(vals, v)
		}
		sort.Slice(vals, func(i, j int) bool { return len(byValue[vals[i]]) > len(byValue[vals[j]]) })
		for _, v := range vals {
			nodes := byValue[v]
			sort.Strings(nodes)
			fmt.Printf("  %s  (%s)\n", v, strings.Join(nodes, ", "))
		}
	}
	if differing == 0 {
		fmt.Printf("# %d instances, config identical\n", len(insts))
	} else {
		fmt.Printf("# %d key(s) differ across %d instances\n", differing, len(insts))
	}
	return nil
}

// flattenConfig flattens a nested config into dot-separated key paths, each with
// a compact-JSON leaf value. Nested maps recurse; anything else (scalar, array)
// is a leaf.
func flattenConfig(prefix string, v any, out map[string]string) {
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		for k, val := range m {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenConfig(key, val, out)
		}
		return
	}
	b, err := json.Marshal(normalizeNumbers(v))
	if err != nil {
		b = fmt.Append(nil, v)
	}
	out[prefix] = string(b)
}

// renderConfig prints one instance's config JSON as TOML (default) or indented
// JSON.
func renderConfig(configJSON, output string) error {
	switch output {
	case "json":
		var buf bytes.Buffer
		if json.Indent(&buf, []byte(configJSON), "", "  ") == nil {
			fmt.Println(buf.String())
		} else {
			fmt.Println(configJSON)
		}
	case "toml", "":
		out, err := configToTOML(configJSON)
		if err != nil {
			return err
		}
		fmt.Print(out)
	default:
		return fmt.Errorf("unknown -o %q (use toml or json)", output)
	}
	return nil
}

// configToTOML renders a JSON config object as TOML — the format reva admins
// read — keeping integers as integers (not 15.0).
func configToTOML(jsonStr string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(jsonStr))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return "", fmt.Errorf("decoding config: %w", err)
	}
	normalizeNumbers(m)
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return "", fmt.Errorf("encoding toml: %w", err)
	}
	return buf.String(), nil
}

// normalizeNumbers turns json.Number into int64 (when integral) or float64 so
// TOML shows 15 rather than 15.0; it mutates maps/slices in place.
func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeNumbers(val)
		}
	case []any:
		for i, e := range t {
			t[i] = normalizeNumbers(e)
		}
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	}
	return v
}
