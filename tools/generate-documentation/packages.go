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
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const module = "github.com/cs3org/reva/v3"

const (
	kindGRPCService     = "grpc-services"
	kindGRPCInterceptor = "grpc-interceptors"
	kindHTTPService     = "http-services"
	kindHTTPMiddleware  = "http-middlewares"
	kindServerless      = "serverless-services"
	kindDriver          = "drivers"
	kindLibrary         = "libraries"
)

type registration struct {
	Kind     string
	Registry string // directory of the registry package, e.g. pkg/storage/fs/registry
	Name     string
}

// namespace feeds a registry: drivers that external plugins register under
// the namespace land in that registry.
type namespace struct {
	Name     string
	Registry string
}

type pkgInfo struct {
	Dir        string // relative to the repository root
	Doc        string
	Readme     string   // without the header
	Name       string   // subsystem_name in the README header
	Category   string   // subsystem_category in the README header
	Imports    []string // directories of the in-module packages it imports
	Regs       []registration
	Namespaces []namespace
	BuildTags  []string
	Config     []configStruct
}

// registryKinds names the registries that do not hold drivers.
var registryKinds = map[string]map[string]string{
	"pkg/rgrpc": {
		"Register":                 kindGRPCService,
		"RegisterUnaryInterceptor": kindGRPCInterceptor,
	},
	"pkg/rhttp/global": {
		"Register":           kindHTTPService,
		"RegisterMiddleware": kindHTTPMiddleware,
	},
	"pkg/rserverless": {
		"Register": kindServerless,
	},
}

func loadPackages(roots ...string) (map[string]*pkgInfo, error) {
	pkgs := map[string]*pkgInfo{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			if name := d.Name(); name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			info, err := loadPackage(filepath.ToSlash(p))
			if info != nil {
				pkgs[info.Dir] = info
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return pkgs, nil
}

func loadPackage(dir string) (*pkgInfo, error) {
	fset := token.NewFileSet()
	files, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil || len(files) == 0 {
		return nil, err
	}

	// Outside tests a directory holds one package. Sorting keeps the output
	// stable if it ever holds more.
	pkg := files[slices.Sorted(maps.Keys(files))[0]]

	info := &pkgInfo{Dir: dir}
	imports := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(pkg.Files)) {
		f := pkg.Files[name]
		if f.Doc != nil && (info.Doc == "" || filepath.Base(name) == "doc.go") {
			info.Doc = f.Doc.Text()
		}

		// Only custom tags are reported: they decide whether a driver is in a
		// build. Platform and toolchain tags do not.
		for _, cg := range f.Comments {
			if cg.Pos() > f.Package {
				break
			}
			for _, c := range cg.List {
				expr, err := constraint.Parse(c.Text)
				if err != nil {
					continue
				}
				tag, ok := expr.(*constraint.TagExpr)
				if !ok || strings.HasPrefix(tag.Tag, "go1.") || slices.Contains(info.BuildTags, tag.Tag) {
					continue
				}
				switch tag.Tag {
				case "linux", "darwin", "windows", "freebsd", "unix", "cgo", "amd64", "arm64", "ignore", "tools":
					continue
				}
				info.BuildTags = append(info.BuildTags, tag.Tag)
			}
		}

		aliases := fileImports(f)
		for _, dir := range aliases {
			imports[dir] = true
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "init" || fn.Recv != nil || fn.Body == nil {
				continue
			}
			for _, stmt := range fn.Body.List {
				es, ok := stmt.(*ast.ExprStmt)
				if !ok {
					continue
				}
				call, ok := es.X.(*ast.CallExpr)
				if !ok {
					continue
				}
				if reg, ok := registrationOf(call, aliases); ok {
					// A driver can register from two files, one per build tag.
					if !slices.Contains(info.Regs, reg) {
						info.Regs = append(info.Regs, reg)
					}
				} else if ns, ok := namespaceOf(call, aliases); ok {
					info.Namespaces = append(info.Namespaces, ns)
				}
			}
		}
	}

	info.Config = extractConfig(fset, pkg.Files)
	info.Imports = slices.Sorted(maps.Keys(imports))
	if b, err := os.ReadFile(path.Join(dir, "README.md")); err == nil {
		info.Readme = string(b)
		if rest, ok := strings.CutPrefix(info.Readme, "---\n"); ok {
			if header, body, ok := strings.Cut(rest, "\n---\n"); ok {
				var h struct {
					Name     string `yaml:"subsystem_name"`
					Category string `yaml:"subsystem_category"`
				}
				if err := yaml.Unmarshal([]byte(header), &h); err != nil {
					return nil, fmt.Errorf("%s/README.md: %w", dir, err)
				}
				info.Name, info.Category, info.Readme = h.Name, h.Category, body
			}
		}
	}
	return info, nil
}

// fileImports maps the names a file uses for in-module packages to their
// directories.
func fileImports(f *ast.File) map[string]string {
	m := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || !strings.HasPrefix(p, module+"/") {
			continue
		}
		dir := strings.TrimPrefix(p, module+"/")
		name := path.Base(dir)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			m[name] = dir
		}
	}
	return m
}

// selectorCall returns the package directory and function name of a call
// pkg.Func("name", ...) to an in-module package, and its first argument.
func selectorCall(call *ast.CallExpr, aliases map[string]string) (dir, fn, arg string, ok bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", "", false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", "", false
	}
	if dir, ok = aliases[x.Name]; !ok {
		return "", "", "", false
	}
	if len(call.Args) > 0 {
		if lit, isLit := call.Args[0].(*ast.BasicLit); isLit && lit.Kind == token.STRING {
			arg, _ = strconv.Unquote(lit.Value)
		}
	}
	return dir, sel.Sel.Name, arg, true
}

func registrationOf(call *ast.CallExpr, aliases map[string]string) (registration, bool) {
	dir, fn, name, ok := selectorCall(call, aliases)
	if !ok || name == "" || !strings.HasPrefix(fn, "Register") {
		return registration{}, false
	}
	if kinds, special := registryKinds[dir]; special {
		kind, ok := kinds[fn]
		return registration{Kind: kind, Registry: dir, Name: name}, ok
	}
	return registration{Kind: kindDriver, Registry: dir, Name: name}, fn == "Register"
}

// namespaceOf recognises plugin.RegisterNamespace("ns", func(...) { registry.Register(...) }).
func namespaceOf(call *ast.CallExpr, aliases map[string]string) (namespace, bool) {
	dir, fn, name, ok := selectorCall(call, aliases)
	if !ok || dir != "pkg/plugin" || fn != "RegisterNamespace" || name == "" || len(call.Args) != 2 {
		return namespace{}, false
	}
	lit, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return namespace{}, false
	}
	ns := namespace{Name: name}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if d, f, _, ok := selectorCall(c, aliases); ok && f == "Register" {
				ns.Registry = d
			}
		}
		return ns.Registry == ""
	})
	return ns, ns.Registry != ""
}
