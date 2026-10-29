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

package revacmd

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/output"
	"github.com/spf13/cobra"
)

type findFilter struct {
	name                 string
	glob                 bool
	sizeOp               byte
	size                 int64
	newerThan, olderThan time.Time
	dirs, files          bool
}

func newFindFilter(name, size, newer, older, kind string) (findFilter, error) {
	f := findFilter{name: strings.ToLower(name), glob: strings.ContainsAny(name, "*?[")}
	if f.glob {
		if _, err := path.Match(f.name, "probe"); err != nil {
			return f, cli.Usagef("invalid --name pattern %q: %v", name, err)
		}
	}
	if size != "" {
		op, rest := byte('='), size
		if size[0] == '+' || size[0] == '-' {
			op, rest = size[0], size[1:]
		}
		n, err := cli.ParseSize(rest)
		if err != nil || n < 0 {
			return f, cli.Usagef("invalid --size %q: use something like +1G, -10M or 4096", size)
		}
		f.sizeOp, f.size = op, n
	}
	var err error
	if newer != "" {
		if f.newerThan, err = parseWhen(newer); err != nil {
			return f, cli.Usagef("invalid --newer %q: use a span like 7d or a date like 2026-09-01", newer)
		}
	}
	if older != "" {
		if f.olderThan, err = parseWhen(older); err != nil {
			return f, cli.Usagef("invalid --older %q: use a span like 30d or a date like 2026-09-01", older)
		}
	}
	switch strings.ToLower(kind) {
	case "":
	case "f", "file":
		f.files = true
	case "d", "dir", "directory":
		f.dirs = true
	default:
		return f, cli.Usagef("invalid --type %q: use f for files or d for folders", kind)
	}
	if f == (findFilter{}) {
		return f, cli.Usagef("pass something to search for: --name, --size, --newer, --older or --type")
	}
	return f, nil
}

// parseWhen reads a date, or a span back from now, such as 7d, 2w or 36h.
func parseWhen(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	unit, num := 24*time.Hour, s
	switch {
	case strings.HasSuffix(s, "d"):
		num = strings.TrimSuffix(s, "d")
	case strings.HasSuffix(s, "w"):
		num, unit = strings.TrimSuffix(s, "w"), 7*24*time.Hour
	default:
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 {
			return time.Time{}, errors.New("bad span")
		}
		return time.Now().Add(-d), nil
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || n <= 0 {
		return time.Time{}, errors.New("bad span")
	}
	return time.Now().Add(-time.Duration(n * float64(unit))), nil
}

// nameOnly reports whether the search of the server can answer.
func (f findFilter) nameOnly() bool {
	return f.name != "" && !f.glob && f.sizeOp == 0 && f.newerThan.IsZero() && f.olderThan.IsZero() && !f.dirs && !f.files
}

func (f findFilter) matches(i *provider.ResourceInfo) bool {
	dir := i.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER
	if f.dirs && !dir || f.files && dir {
		return false
	}
	if f.name != "" {
		name := strings.ToLower(path.Base(i.Path))
		if ok, _ := path.Match(f.name, name); f.glob && !ok || !f.glob && !strings.Contains(name, f.name) {
			return false
		}
	}
	size := int64(i.Size)
	switch f.sizeOp {
	case '+':
		if size <= f.size {
			return false
		}
	case '-':
		if size >= f.size {
			return false
		}
	case '=':
		if size != f.size {
			return false
		}
	}
	mtime := time.Unix(int64(i.GetMtime().GetSeconds()), 0)
	if !f.newerThan.IsZero() && (i.Mtime == nil || mtime.Before(f.newerThan)) {
		return false
	}
	if !f.olderThan.IsZero() && (i.Mtime == nil || mtime.After(f.olderThan)) {
		return false
	}
	return true
}

var errLimit = errors.New("limit reached")

func kindOf(i *provider.ResourceInfo) string {
	if i.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return "dir"
	}
	return "file"
}

func findCommand(rt *cli.Runtime) *cobra.Command {
	var name, size, newer, older, kind string
	var print0 bool
	var limit int
	cmd := &cobra.Command{
		Use:   "find PATH",
		Short: "Search for files by name, size, age or kind",
		Long: "Search a folder and everything below it.\n\n" +
			"--name matches a glob when it has * or ?, else text anywhere in the name.\n" +
			"--size, --newer, --older and --type narrow it further, and each one works on\n" +
			"its own. The server can search only by name, so anything else walks the tree.",
		Example: "  reva find home: --name report\n" +
			"  reva find home: --name '*.root' --size +1G\n" +
			"  reva find home:data --newer 7d --type f --print0 | xargs -0 -n1 echo",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			f, err := newFindFilter(name, size, newer, older, kind)
			if err != nil {
				return err
			}
			if print0 && rt.Output() != "table" {
				return cli.Usagef("--print0 writes bare paths: it does not go with --output %s", rt.Output())
			}
			root, err := rt.RemotePath(ctx, args[0])
			if err != nil {
				return err
			}
			var found []*provider.ResourceInfo
			err = backend.ErrNotAvailable
			if f.nameOnly() {
				var s backend.Searcher
				if s, err = cli.Ability[backend.Searcher](ctx, rt, "search"); err == nil {
					found, err = s.Search(ctx, pathRef(root), backend.SearchOptions{Pattern: name, Limit: limit})
				}
				if errors.Is(err, backend.ErrNotAvailable) {
					rt.Msg(cmd.ErrOrStderr(), "The server cannot search: looking through the files instead...")
				}
			}
			if errors.Is(err, backend.ErrNotAvailable) {
				b, berr := rt.Backend(ctx)
				if berr != nil {
					return berr
				}
				found = nil
				err = b.Files().Walk(ctx, pathRef(root), func(i *provider.ResourceInfo) error {
					if path.Clean(i.Path) != path.Clean(root) && f.matches(i) {
						found = append(found, i)
					}
					if limit > 0 && len(found) >= limit {
						return errLimit
					}
					return nil
				})
				if errors.Is(err, errLimit) {
					err = nil
				}
			}
			if err != nil {
				return err
			}
			if print0 {
				for _, i := range found {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\x00", i.Path)
				}
				return nil
			}
			t := cli.Table{Headers: []string{"TYPE", "SIZE", "PATH"}, Items: found}
			for _, i := range found {
				t.Rows = append(t.Rows, []string{kindOf(i), output.HumanSize(int64(i.Size)), i.Path})
			}
			if err := rt.Print(cmd.OutOrStdout(), t, nil); err != nil {
				return err
			}
			if limit > 0 && len(found) >= limit {
				rt.Msg(cmd.ErrOrStderr(), "Stopped at %d results, which is the --limit.", limit)
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&name, "name", "", "glob with * or ?, else text anywhere in the name")
	fl.StringVar(&size, "size", "", "size to match: +1G larger, -10M smaller, 1G exactly")
	fl.StringVar(&newer, "newer", "", "changed within this span, for example 7d, or since a date")
	fl.StringVar(&older, "older", "", "not changed for this long, for example 30d, or before a date")
	fl.StringVar(&kind, "type", "", "only files (f) or only folders (d)")
	fl.BoolVar(&print0, "print0", false, "write bare paths separated by NUL, for xargs -0")
	fl.IntVar(&limit, "limit", 200, "maximum number of results (0 for none)")
	return cmd
}

type duEntry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Dir   bool   `json:"dir"`
}

func duCommand(rt *cli.Runtime) *cobra.Command {
	var depth, top int
	var human, summarize bool
	cmd := &cobra.Command{
		Use:   "du [PATH...]",
		Short: "Show how much space a folder uses",
		Long: "Show the space used: a size, a tab and a path, like du. Only the total of\n" +
			"each path, unless -d asks for the folders below it. --top ranks everything\n" +
			"below by size and shows the biggest, which is the question to ask when a\n" +
			"quota is full.",
		Example: "  reva du -h home:\n" +
			"  reva du -h -d 1 home:\n" +
			"  reva du -h --top 20 home:",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeRemotePath(rt, math.MaxInt),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if len(args) == 0 {
				args = []string{"home:"}
			}
			switch {
			case summarize && top > 0:
				return cli.Usagef("-s shows only the total of each path: there is nothing for --top to rank")
			case top < 0:
				return cli.Usagef("--top takes a positive number")
			case summarize:
				depth = 0
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			var all []duEntry
			for _, a := range args {
				root, err := rt.RemotePath(ctx, a)
				if err != nil {
					return err
				}
				entries, err := usage(ctx, b, root, depth, top > 0)
				if err != nil {
					return err
				}
				all = append(all, entries...)
			}
			if top > 0 {
				slices.SortStableFunc(all, func(x, y duEntry) int { return cmp.Compare(y.Bytes, x.Bytes) })
				all = all[:min(top, len(all))]
			}
			size := func(n int64) string {
				if human {
					return output.HumanSize(n)
				}
				return strconv.FormatInt(n, 10)
			}
			t := cli.Table{Headers: []string{"SIZE", "PATH"}, Items: all}
			for _, e := range all {
				t.Rows = append(t.Rows, []string{size(e.Bytes), e.Path})
			}
			return rt.Print(cmd.OutOrStdout(), t, func(w io.Writer) {
				for _, e := range all {
					fmt.Fprintf(w, "%s\t%s\n", size(e.Bytes), e.Path)
				}
			})
		},
	}
	cmd.Flags().IntVarP(&depth, "max-depth", "d", -1, "show the folders this many levels below each path (-1: only the totals, or all with --top)")
	cmd.Flags().IntVar(&top, "top", 0, "show only the N biggest entries below the paths")
	cmd.Flags().BoolVarP(&human, "human-readable", "h", false, "sizes like 1.5G")
	cmd.Flags().BoolVarP(&summarize, "summarize", "s", false, "only the total of each path")
	cmd.Flags().BoolP("help", "", false, "help for du")
	return cmd
}

// usage returns the size of root and of the entries below it, to depth. The
// server gives the size of a folder, so only the levels that show are listed.
// A ranking walks the whole tree.
func usage(ctx context.Context, b backend.Backend, root string, depth int, rank bool) ([]duEntry, error) {
	info, err := b.Files().Stat(ctx, pathRef(root))
	if err != nil {
		return nil, err
	}
	out := []duEntry{{Path: info.Path, Bytes: int64(info.Size), Dir: info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER}}
	if rank {
		out = nil
	}
	var visit func(p string, level int) error
	visit = func(p string, level int) error {
		if depth >= 0 && level > depth {
			return nil
		}
		children, err := b.Files().List(ctx, pathRef(p))
		if err != nil {
			return err
		}
		for _, c := range children {
			dir := c.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER
			if dir || rank {
				out = append(out, duEntry{Path: c.Path, Bytes: int64(c.Size), Dir: dir})
			}
			if dir {
				if err := visit(c.Path, level+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if out != nil && depth < 1 {
		return out, nil
	}
	if out == nil && depth == 0 {
		depth = -1
	}
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if err := visit(info.Path, 1); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func tailCommand(rt *cli.Runtime) *cobra.Command {
	var lines int
	var bytesN int64
	var follow bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "tail PATH",
		Short: "Show the end of a file, and follow it as it grows",
		Long: "Show the last lines of a file. -f keeps going, and prints what is added as\n" +
			"the file grows: the server is asked for the size every --interval, and only\n" +
			"the new bytes are read, with a byte range.",
		Example: "  reva tail home:logs/job.log\n" +
			"  reva tail -f -n 50 home:logs/job.log",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p, err := rt.RemotePath(ctx, args[0])
			if err != nil {
				return err
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			d, err := rt.Data(ctx)
			if err != nil {
				return err
			}
			info, err := b.Files().Stat(ctx, pathRef(p))
			if err != nil {
				return err
			}
			if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
				return cli.Usagef("%s: is a folder", p)
			}
			size := int64(info.Size)
			// Read a window from the end. It grows until it holds enough lines.
			start := size
			for window := int64(64 << 10); ; window *= 4 {
				from := max(size-window, 0)
				if bytesN > 0 {
					from = max(size-bytesN, 0)
				}
				body, _, err := d.Download(ctx, pathRef(p), from)
				if err != nil {
					return err
				}
				data, err := io.ReadAll(io.LimitReader(body, size-from))
				body.Close()
				if err != nil {
					return err
				}
				if bytesN > 0 {
					if _, err := cmd.OutOrStdout().Write(data); err != nil {
						return err
					}
					start = from + int64(len(data))
					break
				}
				if cut := lastLines(data, lines); cut >= 0 || from == 0 {
					_, err = cmd.OutOrStdout().Write(data[max(cut, 0):])
					start = from + int64(len(data))
					if err != nil {
						return err
					}
					break
				}
			}
			for follow {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(interval):
				}
				info, err := b.Files().Stat(ctx, pathRef(p))
				if err != nil {
					return err
				}
				switch now := int64(info.Size); {
				case now < start:
					rt.Msg(cmd.ErrOrStderr(), "%s: the file was truncated", p)
					start = 0
				case now == start:
					continue
				}
				body, _, err := d.Download(ctx, pathRef(p), start)
				if err != nil {
					return err
				}
				n, err := io.Copy(cmd.OutOrStdout(), body)
				body.Close()
				if err != nil {
					return err
				}
				start += n
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 10, "number of lines to show")
	cmd.Flags().Int64VarP(&bytesN, "bytes", "c", 0, "show the last N bytes instead of lines")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing what is added to the file")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "how often -f asks for the size")
	return cmd
}

// lastLines returns where the last n lines of data start, or -1 if there are
// fewer.
func lastLines(data []byte, n int) int {
	end := len(data)
	if end > 0 && data[end-1] == '\n' {
		end--
	}
	for range n {
		j := bytes.LastIndexByte(data[:end], '\n')
		if j < 0 {
			return -1
		}
		end = j
	}
	return end + 1
}

func editCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit PATH",
		Short: "Edit a remote file with your editor",
		Long: "Download a file, open it in $VISUAL or $EDITOR, and upload it when the editor\n" +
			"exits, if it changed. The upload is refused when somebody else changed the file\n" +
			"in the meantime: your copy is then kept, and its path is printed. A file that\n" +
			"does not exist is created.",
		Example:           "  reva edit home:notes.md",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p, err := rt.RemotePath(ctx, args[0])
			if err != nil {
				return err
			}
			editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
			if editor == "" {
				return cli.Usagef("set $VISUAL or $EDITOR to the editor to use")
			}
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			d, err := rt.Data(ctx)
			if err != nil {
				return err
			}
			var before []byte
			etag := ""
			info, err := b.Files().Stat(ctx, pathRef(p))
			switch {
			case cli.IsNotFound(err):
			case err != nil:
				return err
			case info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER:
				return cli.Usagef("%s: is a folder", p)
			default:
				etag = info.Etag
				body, _, err := d.Download(ctx, pathRef(p), 0)
				if err != nil {
					return err
				}
				before, err = io.ReadAll(body)
				body.Close()
				if err != nil {
					return err
				}
			}
			dir, err := os.MkdirTemp("", rt.Name()+"-edit-")
			if err != nil {
				return err
			}
			local := filepath.Join(dir, path.Base(p))
			if err := os.WriteFile(local, before, 0o600); err != nil {
				return err
			}
			fields := strings.Fields(editor)
			ed := exec.CommandContext(ctx, fields[0], append(fields[1:], local)...)
			ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := ed.Run(); err != nil {
				return fmt.Errorf("%s: %w (your copy is in %s)", fields[0], err, local)
			}
			after, err := os.ReadFile(local)
			if err != nil {
				return err
			}
			if bytes.Equal(before, after) && etag != "" {
				os.RemoveAll(dir)
				rt.Msg(cmd.ErrOrStderr(), "No change to %s.", p)
				return nil
			}
			open := func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(after)), nil }
			if etag == "" {
				err = d.Upload(ctx, pathRef(p), open, int64(len(after)), "")
			} else {
				err = d.UploadIfUnchanged(ctx, pathRef(p), open, int64(len(after)), etag)
			}
			if se, ok := errors.AsType[*cli.StatusError](err); ok && se.Code == rpc.Code_CODE_FAILED_PRECONDITION {
				return fmt.Errorf("%s changed on the server while you edited it: your copy is in %s", p, local)
			}
			if err != nil {
				return fmt.Errorf("%w (your copy is in %s)", err, local)
			}
			os.RemoveAll(dir)
			rt.Msg(cmd.ErrOrStderr(), "Saved %s (%s).", p, output.HumanSize(int64(len(after))))
			return nil
		},
	}
	return cmd
}
