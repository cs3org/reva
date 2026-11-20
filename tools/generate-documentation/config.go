// Copyright 2018-2024 CERN
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

package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type configStruct struct {
	Name   string        `json:"name"`
	Fields []configField `json:"fields"`
}

type configField struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // file#L<line>
	Embeds      string `json:"embeds,omitempty"`
}

// sharedDefaults are the getters that fall back to the [shared] section.
var sharedDefaults = map[string]string{
	"sharedconf.GetGatewaySVC":  "shared gatewaysvc",
	"sharedconf.GetJWTSecret":   "shared jwt_secret",
	"sharedconf.GetDataGateway": "shared datagateway",
}

// refPrefix marks a default copied from another field. It is resolved to that
// field's key when all keys of the struct are known.
const refPrefix = "\x00field:"

var typeNames = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// extractConfig returns the structs of a package that have mapstructure tags,
// the root config struct first.
func extractConfig(fset *token.FileSet, files map[string]*ast.File) []configStruct {
	defaults := map[string]map[string]string{}
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "ApplyDefaults" || fn.Body == nil ||
				len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Names[0].Name
			typ := strings.TrimPrefix(types.ExprString(fn.Recv.List[0].Type), "*")
			if defaults[typ] == nil {
				defaults[typ] = map[string]string{}
			}
			collectDefaults(fn.Body, recv, defaults[typ])
		}
	}

	byName := map[string]configStruct{}
	root := ""
	for name, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				cs := configStruct{Name: ts.Name.Name}
				keys := map[string]string{}
				for _, field := range st.Fields.List {
					fields := fieldsOf(fset, name, field, defaults[ts.Name.Name])
					for i, n := range field.Names {
						if i < len(fields) {
							keys[n.Name] = fields[i].Key
						}
					}
					cs.Fields = append(cs.Fields, fields...)
				}
				for i, f := range cs.Fields {
					if ref, ok := strings.CutPrefix(f.Default, refPrefix); ok {
						cs.Fields[i].Default = ""
						if key, ok := keys[ref]; ok {
							cs.Fields[i].Default = "same as " + key
						}
					}
				}
				if len(cs.Fields) > 0 {
					byName[cs.Name] = cs
					if strings.EqualFold(cs.Name, "config") {
						root = cs.Name
					}
				}
			}
		}
	}

	// Other structs with mapstructure tags, such as request payloads, are not
	// configuration. Keep only what the root struct reaches. A package
	// without a root struct keeps all of them.
	seen := map[string]bool{}
	if root == "" {
		for name := range byName {
			seen[name] = true
		}
	} else {
		seen[root] = true
		queue := []string{root}
		for len(queue) > 0 {
			cs := byName[queue[0]]
			queue = queue[1:]
			for _, f := range cs.Fields {
				for _, name := range typeNames.FindAllString(f.Type+" "+f.Embeds, -1) {
					if _, ok := byName[name]; ok && !seen[name] {
						seen[name] = true
						queue = append(queue, name)
					}
				}
			}
		}
	}

	var structs []configStruct
	if root != "" {
		structs = append(structs, byName[root])
	}
	for _, name := range slices.Sorted(maps.Keys(seen)) {
		if name != root {
			structs = append(structs, byName[name])
		}
	}
	return structs
}

// collectDefaults records the values ApplyDefaults assigns to the receiver's
// fields, as written in the source.
func collectDefaults(body *ast.BlockStmt, recv string, out map[string]string) {
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for i, lhs := range as.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != recv {
				continue
			}
			v := types.ExprString(as.Rhs[i])
			switch rhs := as.Rhs[i].(type) {
			case *ast.CallExpr:
				if shared, ok := sharedDefaults[types.ExprString(rhs.Fun)]; ok {
					v = shared
				}
			case *ast.SelectorExpr:
				if x, ok := rhs.X.(*ast.Ident); ok && x.Name == recv {
					v = refPrefix + rhs.Sel.Name
				}
			}
			// Later assignments to the same field are usually derived values.
			if _, seen := out[sel.Sel.Name]; !seen && len(v) <= 80 {
				out[sel.Sel.Name] = v
			}
		}
		return true
	})
}

func fieldsOf(fset *token.FileSet, file string, field *ast.Field, defaults map[string]string) []configField {
	if field.Tag == nil {
		return nil
	}
	tagValue, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return nil
	}
	tag := reflect.StructTag(tagValue)
	ms, ok := tag.Lookup("mapstructure")
	if !ok {
		return nil
	}
	key, opts, _ := strings.Cut(ms, ",")
	if key == "-" {
		return nil
	}

	base := configField{
		Type:   types.ExprString(field.Type),
		Source: file + "#L" + strconv.Itoa(fset.Position(field.Pos()).Line),
	}
	if strings.Contains(opts, "squash") {
		base.Embeds = strings.TrimPrefix(base.Type, "*")
		return []configField{base}
	}

	desc := strings.TrimSpace(field.Doc.Text())
	if desc == "" {
		desc = strings.TrimSpace(field.Comment.Text())
	}
	if docs, ok := tag.Lookup("docs"); ok {
		def, d, _ := strings.Cut(docs, ";")
		if desc == "" {
			desc = strings.TrimSpace(d)
		}
		// "url:" is a cato directive, not a value.
		if def = strings.TrimSpace(def); def != "" && def != "-" && def != "nil" && !strings.HasPrefix(def, "url:") {
			base.Default = def
		}
	}
	// TODO and FIXME notes are for developers, not for operators.
	lines := strings.Split(desc, "\n")
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "TODO") || strings.HasPrefix(t, "FIXME") {
			desc = strings.TrimSpace(strings.Join(lines[:i], "\n"))
			break
		}
	}
	base.Description = desc

	var out []configField
	for _, name := range field.Names {
		f := base
		f.Key = key
		if f.Key == "" {
			f.Key = strings.ToLower(name.Name)
		}
		if d, ok := defaults[name.Name]; ok {
			f.Default = d
		}
		out = append(out, f)
	}
	return out
}
