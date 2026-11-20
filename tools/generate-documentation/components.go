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
	"go/doc"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	repoURL   = "https://github.com/cs3org/reva"
	treeURL   = repoURL + "/tree/master/"
	blobURL   = repoURL + "/blob/master/"
	rawURL    = "https://raw.githubusercontent.com/cs3org/reva/master/"
	godocURL  = "https://pkg.go.dev/"
	sectionID = "docs/components"
)

type kindInfo struct {
	Title       string
	Description string
	Weight      int
	// Table is the TOML table a component of this kind is configured under,
	// with %s for its name.
	Table string
}

var kinds = map[string]kindInfo{
	kindGRPCService: {
		Title:       "gRPC services",
		Description: "Services that implement the CS3 APIs. The gateway is the entry point; the others are providers it routes to.",
		Weight:      10,
		Table:       "grpc.services.%s",
	},
	kindHTTPService: {
		Title:       "HTTP services",
		Description: "Services that expose reva over HTTP: WebDAV, OCS, Graph, OCM, data transfer, and more.",
		Weight:      20,
		Table:       "http.services.%s",
	},
	kindHTTPMiddleware: {
		Title:       "HTTP middlewares",
		Description: "Handlers that wrap every HTTP request: authentication, CORS, logging, tracing.",
		Weight:      30,
		Table:       "http.middlewares.%s",
	},
	kindGRPCInterceptor: {
		Title:       "gRPC interceptors",
		Description: "Handlers that wrap every gRPC call.",
		Weight:      40,
		Table:       "grpc.interceptors.%s",
	},
	kindServerless: {
		Title:       "Serverless services",
		Description: "Services that run in the background without a listener: jobs, notifications, reconciliation.",
		Weight:      50,
		Table:       "serverless.services.%s",
	},
	kindDriver: {
		Title:       "Drivers",
		Description: "Pluggable back ends. A service picks one driver per family in its configuration.",
		Weight:      60,
	},
	kindLibrary: {
		Title:       "Libraries",
		Description: "Shared packages that are not plugins but have their own documentation or configuration.",
		Weight:      70,
	},
}

type component struct {
	pkg      *pkgInfo
	Kind     string
	Name     string
	Title    string
	Category string
	Family   *family // drivers only
	Builtin  bool    // configured under its kind's table but not registered as a plugin
	URL      string
}

type family struct {
	Registry   string
	Title      string
	Category   string
	Slug       string
	Namespaces []string
	Members    []*component
	URL        string
}

type catalog struct {
	pkgs       map[string]*pkgInfo
	components []*component
	families   map[string]*family
	libraries  []*pkgInfo // packages with a doc comment and no page
}

func buildCatalog(pkgs map[string]*pkgInfo) *catalog {
	c := &catalog{pkgs: pkgs, families: map[string]*family{}}
	dirs := slices.Sorted(maps.Keys(pkgs))

	for _, dir := range dirs {
		for _, ns := range pkgs[dir].Namespaces {
			if _, special := registryKinds[ns.Registry]; special {
				continue
			}
			f := c.family(ns.Registry)
			f.Namespaces = append(f.Namespaces, ns.Name)
		}
	}

	registries := map[string]bool{}
	for _, p := range pkgs {
		for _, r := range p.Regs {
			if r.Kind == kindDriver {
				registries[r.Registry] = true
			}
		}
	}

	used := map[string]bool{}
	for _, dir := range dirs {
		p := pkgs[dir]
		parent, base := path.Split(dir)
		var comp *component
		switch {
		case len(p.Regs) > 0:
			r := p.Regs[0]
			comp = &component{pkg: p, Kind: r.Kind, Name: r.Name}
			if r.Kind == kindDriver {
				comp.Family = c.family(r.Registry)
				comp.Family.Members = append(comp.Family.Members, comp)
			}
		case parent == "internal/http/interceptors/" && base != "loader" && base != "plugins":
			// The auth, log, and trace middlewares are wired into the runtime,
			// not registered, but they are configured like the others.
			comp = &component{pkg: p, Kind: kindHTTPMiddleware, Name: base, Builtin: true}
		case parent == "internal/grpc/interceptors/" && base != "loader":
			comp = &component{pkg: p, Kind: kindGRPCInterceptor, Name: base, Builtin: true}
		case registries[dir]:
			// The family page shows the registry's README.
			continue
		case p.Readme != "" || len(p.Config) > 0 && p.Doc != "":
			comp = &component{pkg: p, Kind: kindLibrary, Name: base, Title: strings.TrimPrefix(dir, "pkg/")}
		case p.Doc != "":
			c.libraries = append(c.libraries, p)
			continue
		default:
			continue
		}
		switch {
		case p.Name != "":
			comp.Title = p.Name
		case comp.Title == "":
			comp.Title = comp.Name
		}
		comp.Category = p.Category
		if comp.Category == "" && comp.Family != nil {
			comp.Category = comp.Family.Category
		}
		c.components = append(c.components, comp)
	}

	for _, f := range c.families {
		f.URL = "/" + sectionID + "/drivers/" + f.Slug + "/"
	}
	for _, comp := range c.components {
		var section, slug string
		switch comp.Kind {
		case kindDriver:
			section, slug = "drivers/"+comp.Family.Slug, comp.Name
		case kindLibrary:
			section, slug = kindLibrary, strings.ReplaceAll(strings.TrimPrefix(comp.pkg.Dir, "pkg/"), "/", "-")
		default:
			section, slug = comp.Kind, comp.Name
		}
		u := "/" + sectionID + "/" + section + "/" + slug + "/"
		// Two packages can register the same name in one section.
		for i := 2; used[u]; i++ {
			u = fmt.Sprintf("/%s/%s/%s-%d/", sectionID, section, slug, i)
		}
		used[u] = true
		comp.URL = u
	}
	return c
}

func (c *catalog) family(registry string) *family {
	if f, ok := c.families[registry]; ok {
		return f
	}
	f := &family{Registry: registry}
	if p := c.pkgs[registry]; p != nil {
		f.Title, f.Category = p.Name, p.Category
	}
	if f.Title == "" {
		f.Title = strings.TrimSuffix(strings.TrimPrefix(registry, "pkg/"), "/registry")
	}
	f.Slug = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(f.Title), "-"), "-")
	c.families[registry] = f
	return f
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// usedBy links the components that import a package, other than the excluded
// ones, such as the drivers that register into it.
func (c *catalog) usedBy(dir string, exclude map[string]bool) []link {
	out := []link{}
	for _, comp := range c.components {
		if !exclude[comp.pkg.Dir] && slices.Contains(comp.pkg.Imports, dir) {
			out = append(out, link{Title: comp.Title, URL: comp.URL})
		}
	}
	return out
}

// synopsis is the first sentence of a doc comment, without the conventional
// "Package x" opening, which the page title already says.
func synopsis(text string) string {
	var p doc.Package
	s := p.Synopsis(text)
	if rest, ok := strings.CutPrefix(s, "Package "); ok {
		if _, after, ok := strings.Cut(rest, " "); ok && after != "" {
			// "Package x is the y" reads as "The y".
			after = strings.TrimPrefix(after, "is ")
			s = strings.ToUpper(after[:1]) + after[1:]
		}
	}
	return s
}

var (
	mdLink    = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)([^)]*)\)`)
	mdHeading = regexp.MustCompile(`(?m)^(#{1,6})\s`)
)

// readmeMarkdown drops the README title, which the page already has, moves its
// headings below the page title, and points relative links at GitHub, because
// the page lives elsewhere than the README.
func readmeMarkdown(dir, text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "# ") {
		_, text, _ = strings.Cut(text, "\n")
	}

	top := 7
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if m := mdHeading.FindStringSubmatch(line); m != nil && !inFence && len(m[1]) < top {
			top = len(m[1])
		}
	}
	if shift := 2 - top; top < 7 && shift != 0 {
		lines := strings.Split(text, "\n")
		inFence = false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
			}
			if m := mdHeading.FindStringSubmatch(line); m != nil && !inFence {
				level := max(1, min(6, len(m[1])+shift))
				lines[i] = strings.Repeat("#", level) + line[len(m[1]):]
			}
		}
		text = strings.Join(lines, "\n")
	}

	return mdLink.ReplaceAllStringFunc(text, func(s string) string {
		m := mdLink.FindStringSubmatch(s)
		target := m[3]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			return s
		}
		base := blobURL
		if m[1] == "!" {
			base = rawURL
		}
		return m[1] + "[" + m[2] + "](" + base + path.Clean(path.Join(dir, target)) + m[4] + ")"
	})
}
