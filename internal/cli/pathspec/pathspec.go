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

// Package pathspec parses the path arguments accepted by the CLI and
// decides, for each one, whether it names a local file or a remote resource.
//
// That decision is the whole reason this package exists. On lxplus a path like
// /eos/user/g/gdelmont is simultaneously a valid CERNBox path and a real local
// FUSE mount point, so "cernbox cp /eos/a /eos/b" cannot be disambiguated by
// looking at the string, and guessing would silently do the wrong thing. The
// CLI therefore splits its commands in two:
//
//   - Namespace commands (ls, rm, share, ...) have no local side at all, so
//     every path they take is remote. Use ParseRemote.
//   - Transfer commands (cp, sync) require the remote side to be marked with a
//     "cb:" prefix or written as a space alias. Use ParseTransfer.
//
// Nothing outside this package may infer local-versus-remote by any other
// means.
package pathspec

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind distinguishes a local filesystem path from a remote path.
type Kind int

const (
	// Local is a path on the machine running the CLI.
	Local Kind = iota
	// Remote is a path on the server.
	Remote
)

func (k Kind) String() string {
	if k == Remote {
		return "remote"
	}
	return "local"
}

// Prefixes recognised at the start of an argument.
const (
	// RemotePrefix forces an argument to be interpreted as a remote path.
	RemotePrefix = "cb:"
	// LocalPrefix forces an argument to be interpreted as a local path. It
	// exists so that a local directory whose name collides with a space alias
	// is still addressable.
	LocalPrefix = "file:"
)

// HomeAlias is the space alias for the caller's own home space. A relative
// remote path is resolved against it.
const HomeAlias = "home"

// aliasRe matches a space alias: a segment, optionally followed by more
// slash-separated segments, as in "home" or "project/cernbox".
//
// The two-character minimum on the first segment is deliberate: it stops a
// Windows drive letter ("C:\Users\...") from being read as a space alias.
var aliasRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

// Spec is a parsed path argument.
type Spec struct {
	// Kind reports whether this is a local or a remote path.
	Kind Kind

	// Path is the cleaned path. For a Remote spec written absolutely it is the
	// absolute CERNBox path ("/eos/user/g/gdelmont/Documents"). For a Remote
	// spec written with a space alias it is the path relative to that space's
	// root, without a leading slash ("Documents"). For a Local spec it is the
	// local path as given, cleaned.
	Path string

	// Space is the space alias the argument was written with ("home",
	// "project/cernbox"), or empty when the argument was an absolute path or a
	// local path. Resolve turns it into an absolute path.
	Space string

	// TrailingSlash records whether the argument ended in a separator. Transfer
	// commands use it the way cp does: "put a.txt cb:/eos/x/dir/" means "into
	// dir", whereas "put a.txt cb:/eos/x/dir" means "as dir".
	TrailingSlash bool

	// Raw is the original argument, used in error messages so the user sees
	// what they typed.
	Raw string
}

// IsRemote reports whether s names a remote resource.
func (s Spec) IsRemote() bool { return s.Kind == Remote }

// IsLocal reports whether s names a local file.
func (s Spec) IsLocal() bool { return s.Kind == Local }

// String renders the spec back into the syntax a user would type.
func (s Spec) String() string {
	var b strings.Builder
	switch {
	case s.Kind == Local:
		b.WriteString(s.Path)
	case s.Space != "":
		b.WriteString(s.Space)
		b.WriteString(":")
		b.WriteString(s.Path)
	default:
		b.WriteString(s.Path)
	}
	if s.TrailingSlash && !strings.HasSuffix(b.String(), "/") {
		b.WriteString("/")
	}
	return b.String()
}

// SpaceResolver maps a space alias to the absolute CERNBox path of that space's
// root. pkg/client implements it against the spaces listing; tests use a stub.
type SpaceResolver interface {
	ResolveSpace(ctx context.Context, alias string) (string, error)
}

// Resolve returns the absolute CERNBox path for a remote spec, consulting r
// only when the spec was written with a space alias. It is an error to call
// Resolve on a local spec.
func (s Spec) Resolve(ctx context.Context, r SpaceResolver) (string, error) {
	if s.Kind != Remote {
		return "", fmt.Errorf("%q is a local path, it has no remote location", s.Raw)
	}
	if s.Space == "" {
		return s.Path, nil
	}
	if r == nil {
		return "", fmt.Errorf("cannot resolve %q: no space resolver available", s.Raw)
	}
	root, err := r.ResolveSpace(ctx, s.Space)
	if err != nil {
		return "", fmt.Errorf("cannot resolve space %q from %q: %w", s.Space, s.Raw, err)
	}
	if s.Path == "" || s.Path == "." {
		return cleanAbs(root), nil
	}
	return cleanAbs(path.Join(root, s.Path)), nil
}

// ParseRemote parses an argument for a command that only ever operates on
// CERNBox. Every form is accepted: an absolute path, a space alias, a "cb:"
// prefixed path, and a relative path, which is taken relative to the home
// space.
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

	// A bare relative path is relative to the caller's home space, so that
	// "cernbox ls Documents" does what it looks like it does.
	cleaned, err := cleanRel(arg)
	if err != nil {
		return Spec{}, fmt.Errorf("%q: %w", raw, err)
	}
	return Spec{Kind: Remote, Space: HomeAlias, Path: cleaned, TrailingSlash: trailing, Raw: raw}, nil
}

// ParseTransfer parses an argument for a command that moves data between the
// local filesystem and CERNBox. The argument is remote only when it says so:
// either a "cb:" prefix or a space alias. Everything else is local, including
// an absolute /eos path, which on lxplus is a real local mount.
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

// localSpec builds the spec for a local path. It is cleaned by the platform's
// rules rather than by CERNBox's, so that on Windows "C:\data\" keeps its
// volume and its trailing separator.
func localSpec(p, raw string) Spec {
	trailing := len(p) > 1 && (strings.HasSuffix(p, "/") || strings.HasSuffix(p, string(os.PathSeparator)))
	return Spec{Kind: Local, Path: filepath.Clean(p), TrailingSlash: trailing, Raw: raw}
}

// ParseTransferPair parses a source and destination and requires exactly one of
// them to be remote. Two local paths are a job for cp(1); two remote paths are
// a server-side copy, which the CLI exposes as "cernbox cp" on bare paths
// rather than through the transfer engine.
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

// SplitForCompletion splits a partially typed argument into the text naming a
// directory and the fragment being typed inside it. The directory part comes
// back exactly as it was typed, prefix and alias included, so that a caller can
// join a candidate name onto it and get something the user could have typed.
//
//	"cb:/eos/user/g/gdelmont/Doc" -> "cb:/eos/user/g/gdelmont/", "Doc"
//	"home:notes/dr"               -> "home:notes/",              "dr"
//	"home:"                       -> "home:",                    ""
//	"Doc"                         -> "",                         "Doc"
//
// The directory part is not itself a valid argument in every case: "cb:" and ""
// name no path at all. Resolve those to the home space, which is what a bare
// relative path means.
func SplitForCompletion(arg string) (dir, frag string) {
	prefix := ""
	if rest, ok := strings.CutPrefix(arg, RemotePrefix); ok {
		prefix, arg = RemotePrefix, rest
	}
	if alias, rest, ok := splitAlias(arg); ok {
		prefix, arg = prefix+alias+":", rest
	}
	if i := strings.LastIndex(arg, "/"); i >= 0 {
		return prefix + arg[:i+1], arg[i+1:]
	}
	return prefix, arg
}

// Join returns a spec for a child of s.
func Join(s Spec, elems ...string) Spec {
	out := s
	if s.Kind == Local {
		out.Path = filepath.Join(append([]string{s.Path}, elems...)...)
	} else {
		out.Path = path.Join(append([]string{s.Path}, elems...)...)
	}
	if s.Kind == Remote && s.Space == "" && !strings.HasPrefix(out.Path, "/") {
		out.Path = "/" + out.Path
	}
	out.TrailingSlash = false
	out.Raw = out.String()
	return out
}

// Base returns the final element of the spec's path. For a space alias with an
// empty path it returns the last segment of the alias, so that
// "cernbox cp cb:project/cernbox: ." lands in a directory named "cernbox".
func Base(s Spec) string {
	if s.Path == "" || s.Path == "." || s.Path == "/" {
		if s.Space != "" {
			return path.Base(s.Space)
		}
		return ""
	}
	if s.Kind == Local {
		return filepath.Base(s.Path)
	}
	return path.Base(s.Path)
}

// splitAlias splits "home:Documents" into ("home", "Documents", true). It
// returns ok=false when arg carries no alias, or when the text before the colon
// is not shaped like one.
func splitAlias(arg string) (alias, rest string, ok bool) {
	i := strings.Index(arg, ":")
	if i <= 0 {
		return "", "", false
	}
	// A slash before the colon means the colon is part of a file name, as in
	// "./weird:name": not an alias separator.
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

// cleanRel cleans a path that is relative to a space root and rejects any
// attempt to climb above it.
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

// cleanAbsChecked cleans an absolute path and rejects one that climbs above the
// namespace root.
func cleanAbsChecked(p string) (string, error) {
	// path.Clean already collapses "/.." to "/", which would silently turn an
	// escaping path into the root, so count the climb before cleaning.
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
