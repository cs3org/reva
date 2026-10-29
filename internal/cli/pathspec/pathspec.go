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

// Package pathspec parses the path arguments of the CLI and tells if each one
// names a local file or a remote resource.
//
// On lxplus, /eos/user/g/gdelmont is both a CERNBox path and a local FUSE
// mount point. The string alone does not tell which one is meant, so the
// commands split in two:
//
//   - Namespace commands (ls, rm, share, ...) have no local side. Every path is
//     remote. Use ParseRemote.
//   - Transfer commands (cp, sync) need a "cb:" prefix or a space alias on the
//     remote side. Use ParseTransfer.
package pathspec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind tells if a path is local or remote.
type Kind int

const (
	Local Kind = iota
	Remote
)

const (
	// RemotePrefix marks a remote path.
	RemotePrefix = "cb:"
	// LocalPrefix marks a local path. It reaches a local directory whose name
	// is also a space alias.
	LocalPrefix = "file:"
)

// HomeAlias is the alias of the home space. A relative remote path is in it.
const HomeAlias = "home"

// aliasRe matches a space alias, such as "home" or "project/cernbox". The first
// segment has two characters at least, so that a Windows drive letter
// ("C:\Users\...") is not an alias.
var aliasRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

// Spec is a parsed path argument.
type Spec struct {
	Kind Kind

	// Path is the cleaned path. A remote path with a space alias is relative to
	// the root of the space, without a leading slash.
	Path string

	// Space is the space alias, or empty for an absolute or a local path.
	Space string

	// TrailingSlash is true if the argument ended in a separator. As in cp,
	// "cb:/eos/x/dir/" means "into dir" and "cb:/eos/x/dir" means "as dir".
	TrailingSlash bool

	// Raw is the argument as typed, for error messages.
	Raw string
}

// IsRemote reports whether s names a remote resource.
func (s Spec) IsRemote() bool { return s.Kind == Remote }

// IsLocal reports whether s names a local file.
func (s Spec) IsLocal() bool { return s.Kind == Local }

// ParseRemote parses an argument of a command that works on remote paths only.
// A relative path is in the home space.
func ParseRemote(arg string) (Spec, error) {
	if arg == "" {
		return Spec{}, fmt.Errorf("empty path")
	}
	raw := arg

	if rest, ok := strings.CutPrefix(arg, LocalPrefix); ok {
		return Spec{}, fmt.Errorf("%q names a local path, but this command only works on remote paths", rest)
	}
	arg = strings.TrimPrefix(arg, RemotePrefix)
	if arg == "" {
		return Spec{}, fmt.Errorf("%q has no path after %q", raw, RemotePrefix)
	}

	trailing := hasTrailingSlash(arg)

	if alias, rest, ok := splitAlias(arg); ok {
		cleaned, err := cleanRel(rest)
		if err != nil {
			return Spec{}, fmt.Errorf("%q: %w", raw, err)
		}
		return Spec{Kind: Remote, Space: alias, Path: cleaned, TrailingSlash: trailing, Raw: raw}, nil
	}

	if strings.HasPrefix(arg, "/") {
		cleaned, err := cleanAbsChecked(arg)
		if err != nil {
			return Spec{}, fmt.Errorf("%q: %w", raw, err)
		}
		return Spec{Kind: Remote, Path: cleaned, TrailingSlash: trailing, Raw: raw}, nil
	}

	cleaned, err := cleanRel(arg)
	if err != nil {
		return Spec{}, fmt.Errorf("%q: %w", raw, err)
	}
	return Spec{Kind: Remote, Space: HomeAlias, Path: cleaned, TrailingSlash: trailing, Raw: raw}, nil
}

// ParseTransfer parses an argument of a command that moves data between the
// local disk and CERNBox. The argument is remote only with a "cb:" prefix or a
// space alias. An absolute /eos path is local.
func ParseTransfer(arg string) (Spec, error) {
	if arg == "" {
		return Spec{}, fmt.Errorf("empty path")
	}
	raw := arg
	trailing := hasTrailingSlash(arg)

	if rest, ok := strings.CutPrefix(arg, LocalPrefix); ok {
		if rest == "" {
			return Spec{}, fmt.Errorf("%q has no path after %q", raw, LocalPrefix)
		}
		return localSpec(rest, raw), nil
	}

	if rest, ok := strings.CutPrefix(arg, RemotePrefix); ok {
		if rest == "" {
			return Spec{}, fmt.Errorf("%q has no path after %q", raw, RemotePrefix)
		}
		s, err := ParseRemote(rest)
		if err != nil {
			return Spec{}, err
		}
		s.Raw = raw
		s.TrailingSlash = trailing
		return s, nil
	}

	if alias, rest, ok := splitAlias(arg); ok {
		cleaned, err := cleanRel(rest)
		if err != nil {
			return Spec{}, fmt.Errorf("%q: %w", raw, err)
		}
		return Spec{Kind: Remote, Space: alias, Path: cleaned, TrailingSlash: trailing, Raw: raw}, nil
	}

	return localSpec(arg, raw), nil
}

// localSpec cleans the path by the rules of the platform, so that on Windows
// "C:\data\" keeps its volume and its trailing separator.
func localSpec(p, raw string) Spec {
	trailing := len(p) > 1 && (strings.HasSuffix(p, "/") || strings.HasSuffix(p, string(os.PathSeparator)))
	return Spec{Kind: Local, Path: filepath.Clean(p), TrailingSlash: trailing, Raw: raw}
}

// ParseTransferPair parses a source and a destination. They must not both be
// local.
func ParseTransferPair(src, dst string) (Spec, Spec, error) {
	s, err := ParseTransfer(src)
	if err != nil {
		return Spec{}, Spec{}, err
	}
	d, err := ParseTransfer(dst)
	if err != nil {
		return Spec{}, Spec{}, err
	}
	if s.IsLocal() && d.IsLocal() {
		return Spec{}, Spec{}, fmt.Errorf(
			"both %q and %q are local paths: mark the remote side with %q, for example %s%s",
			src, dst, RemotePrefix, RemotePrefix, dst)
	}
	return s, d, nil
}

// splitAlias splits "home:Documents" into ("home", "Documents", true).
func splitAlias(arg string) (alias, rest string, ok bool) {
	i := strings.Index(arg, ":")
	if i <= 0 {
		return "", "", false
	}
	// A slash before the colon means a file name like "./weird:name".
	if strings.Contains(arg[:i], "/") && !aliasRe.MatchString(arg[:i]) {
		return "", "", false
	}
	alias = arg[:i]
	if !aliasRe.MatchString(alias) {
		return "", "", false
	}
	return alias, strings.TrimPrefix(arg[i+1:], "/"), true
}

func hasTrailingSlash(arg string) bool {
	return len(arg) > 1 && strings.HasSuffix(arg, "/")
}

// cleanRel cleans a path in a space. It rejects a path that leaves the space.
func cleanRel(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", nil
	}
	cleaned := path.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes the space root")
	}
	if cleaned == "." {
		return "", nil
	}
	return cleaned, nil
}

// cleanAbsChecked cleans an absolute path. It rejects a path above the root.
func cleanAbsChecked(p string) (string, error) {
	// path.Clean turns "/.." into "/", so check before it.
	if escapesRoot(p) {
		return "", fmt.Errorf("path escapes the namespace root")
	}
	return cleanAbs(p), nil
}

func escapesRoot(p string) bool {
	depth := 0
	for seg := range strings.SplitSeq(strings.TrimPrefix(p, "/"), "/") {
		switch seg {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return true
			}
		default:
			depth++
		}
	}
	return false
}

func cleanAbs(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}
