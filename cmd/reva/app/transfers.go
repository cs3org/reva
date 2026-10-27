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

// The commands that come from cernbox-cli and move content: sync, archive,
// cat, and versions restore, download and diff.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/textdiff"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/data/transfer"
	"github.com/cs3org/reva/v3/pkg/cli/output"
	"github.com/spf13/cobra"
)

func syncCommand(rt *cli.Runtime) *cobra.Command {
	var flags transferFlags
	var del, hidden bool
	var exclude []string
	cmd := &cobra.Command{
		Use:   "sync SOURCE DEST",
		Short: "Make one folder match another, one way",
		Long: "Copy a folder so that the destination matches the source, one way only. Use a\n" +
			"desktop sync client if changes on both sides must merge.\n\n" +
			"Mark the remote side with cb: or a space alias, as for cp. Files are compared\n" +
			"by size and time, so unchanged ones are not sent again, and without --delete\n" +
			"sync only adds and updates. --dry-run shows the plan and changes nothing.",
		Example: "  reva sync ./data cb:/home/data\n" +
			"  reva sync cb:/home/data ./data --delete --dry-run\n" +
			"  reva sync ./src home:src --exclude '*.o' --exclude build",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			src, dst, srcRemote, dstRemote, _, err := rt.TransferPaths(ctx, args[0], args[1])
			if err != nil {
				return err
			}
			if srcRemote == dstRemote {
				return cli.Usagef("sync copies between your computer and the server: exactly one side must be remote")
			}
			opts := transfer.SyncOptions{Delete: del, IncludeHidden: hidden, Exclude: exclude, Direction: transfer.Push}
			local, remote := src, dst
			if srcRemote {
				opts.Direction, local, remote = transfer.Pull, dst, src
			}
			if opts.Direction == transfer.Push {
				if err := rt.ConfirmMutation(cmd.CommandPath()); err != nil {
					return err
				}
			}
			e, err := flags.engine(cmd, rt)
			if err != nil {
				return err
			}
			if del && !flags.dryRun {
				rt.Msg(cmd.ErrOrStderr(), "--delete: files that are not in the source are deleted from the destination.")
			}
			stats, err := e.Sync(ctx, local, remote, opts)
			if err != nil {
				return err
			}
			verb := "Mirrored"
			if flags.dryRun {
				verb = "Would mirror"
			}
			rt.Msg(cmd.ErrOrStderr(), "%s %s: %d created, %d updated, %d deleted, %d unchanged (%s) in %s",
				verb, opts.Direction, stats.Created, stats.Updated, stats.Deleted, stats.Skipped,
				output.HumanSize(stats.Bytes), stats.Duration.Round(time.Millisecond))
			if rt.Output() == "json" {
				return rt.Print(cmd.OutOrStdout(), stats, nil)
			}
			return nil
		},
	}
	flags.register(cmd)
	_ = cmd.Flags().MarkHidden("recursive")
	cmd.Flags().BoolVar(&del, "delete", false, "remove the destination entries that the source does not have")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include the entries whose name starts with a dot")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "leave out the entries that match this pattern, such as '*.o' or build (repeatable)")
	return cmd
}

type archiveResult struct {
	Path    string   `json:"path"`
	Format  string   `json:"format"`
	Bytes   int64    `json:"bytes"`
	Sources []string `json:"sources"`
}

func archiveCommand(rt *cli.Runtime) *cobra.Command {
	var format, to string
	var force bool
	cmd := &cobra.Command{
		Use:   "archive PATH...",
		Short: "Download files and folders as one archive",
		Long: "Download as one archive, which the server packs, so that many small files\n" +
			"cost one request rather than one each.\n\n" +
			"Without --to it lands in the current folder, named after what you asked for.\n" +
			"'--to -' sends it to standard output.",
		Example: "  reva archive /home/data\n" +
			"  reva archive --format zip --to notes.zip home:Documents\n" +
			"  reva archive --to - home:Documents | tar -t",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if format != "tar" && format != "zip" {
				return cli.Usagef("unknown format %q: use tar or zip", format)
			}
			paths := make([]string, len(args))
			refs := make([]*provider.Reference, len(args))
			for i, a := range args {
				p, err := rt.RemotePath(ctx, a)
				if err != nil {
					return err
				}
				paths[i], refs[i] = p, pathRef(p)
			}
			dest := archiveDest(to, paths, format)
			d, err := rt.Data(ctx)
			if err != nil {
				return err
			}
			body, err := d.Archive(ctx, refs, format)
			if err != nil {
				return err
			}
			defer body.Close()
			if dest == "-" {
				_, err := io.Copy(cmd.OutOrStdout(), body)
				return err
			}
			mode := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			if !force {
				mode |= os.O_EXCL
			}
			f, err := os.OpenFile(dest, mode, 0o644)
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("%s exists: pass --force to overwrite it, or --to to write somewhere else", dest)
			}
			if err != nil {
				return err
			}
			var w io.Writer = f
			var m *output.Meter
			if rt.ShowProgress() {
				m = output.NewMeter(os.Stderr, filepath.Base(dest), -1)
				w = m.Writer(f)
			}
			n, err := io.Copy(w, body)
			if m != nil {
				m.Stop()
			}
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				os.Remove(dest)
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Wrote %s (%s)", dest, output.HumanSize(n))
			if rt.Output() == "json" {
				return rt.Print(cmd.OutOrStdout(), archiveResult{Path: dest, Format: format, Bytes: n, Sources: paths}, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "tar", "tar or zip")
	cmd.Flags().StringVar(&to, "to", "", "write the archive here, or - for standard output")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite a file that exists")
	return cmd
}

func archiveDest(to string, paths []string, format string) string {
	name := "archive." + format
	if len(paths) == 1 {
		if base := path.Base(paths[0]); base != "/" && base != "." {
			name = base + "." + format
		}
	}
	switch {
	case to == "":
		return name
	case to == "-":
		return to
	}
	if st, err := os.Stat(to); err == nil && st.IsDir() {
		return filepath.Join(to, name)
	}
	return to
}

func catCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "cat PATH...",
		Short:             "Write the content of files to standard output",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeRemotePath(rt, math.MaxInt),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			d, err := rt.Data(ctx)
			if err != nil {
				return err
			}
			for _, a := range args {
				p, err := rt.RemotePath(ctx, a)
				if err != nil {
					return err
				}
				// A download of a folder answers "not implemented": say what cat(1) says.
				if info, err := b.Files().Stat(ctx, pathRef(p)); err == nil && info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					return cli.Usagef("%s: is a folder", p)
				}
				body, _, err := d.Download(ctx, pathRef(p), 0)
				if err != nil {
					return err
				}
				_, err = io.Copy(cmd.OutOrStdout(), body)
				body.Close()
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func versionsRestoreCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "restore PATH KEY",
		Short:             "Make an earlier version the current one",
		Args:              cobra.ExactArgs(2),
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
			if err := b.Versions().Restore(ctx, pathRef(p), args[1]); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Restored version %s of %s", args[1], p)
			return nil
		},
	}
}

func versionsDownloadCommand(rt *cli.Runtime) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "download PATH KEY",
		Short: "Download an earlier version without restoring it",
		Long: "Download an earlier version and leave the current one alone. Without\n" +
			"--output-file the file goes to the current folder as NAME.KEY.",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p, err := rt.RemotePath(ctx, args[0])
			if err != nil {
				return err
			}
			name := path.Base(p) + "." + args[1]
			dst := out
			if dst == "" {
				dst = name
			}
			if st, err := os.Stat(dst); err == nil && st.IsDir() {
				dst = filepath.Join(dst, name)
			}
			d, err := rt.Data(ctx)
			if err != nil {
				return err
			}
			body, _, err := d.DownloadVersion(ctx, pathRef(p), args[1])
			if err != nil {
				return err
			}
			defer body.Close()
			tmp := dst + ".part"
			f, err := os.Create(tmp)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, body)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err == nil {
				err = os.Rename(tmp, dst)
			}
			if err != nil {
				os.Remove(tmp)
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "Wrote %s (%s)", dst, output.HumanSize(n))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output-file", "F", "", "write to this local path")
	return cmd
}

// maxDiffSize keeps a diff in memory.
const maxDiffSize = 16 << 20

func versionsDiffCommand(rt *cli.Runtime) *cobra.Command {
	var context int
	var plain bool
	cmd := &cobra.Command{
		Use:   "diff PATH [KEY] [KEY]",
		Short: "Show what changed between versions of a file",
		Long: "Show what changed, as a unified diff. With one key, that version against the\n" +
			"file as it is now. With two, one against the other. With none, the newest\n" +
			"version against now.",
		Example: "  reva versions diff home:report.md\n" +
			"  reva versions diff home:report.md OLDER NEWER",
		Args:              cobra.RangeArgs(1, 3),
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
			keys := args[1:]
			if len(keys) == 0 {
				vs, err := b.Versions().List(ctx, pathRef(p))
				if err != nil {
					return err
				}
				if len(vs) == 0 {
					return cli.Usagef("%s has no earlier versions", p)
				}
				newest := vs[0]
				for _, v := range vs {
					if v.Mtime > newest.Mtime {
						newest = v
					}
				}
				keys = []string{newest.Key}
			}
			read := func(key string) (string, string, error) {
				var body io.ReadCloser
				var err error
				label := path.Base(p) + " (now)"
				if key == "" {
					body, _, err = d.Download(ctx, pathRef(p), 0)
				} else {
					label = path.Base(p) + " (" + key + ")"
					body, _, err = d.DownloadVersion(ctx, pathRef(p), key)
				}
				if err != nil {
					return "", "", err
				}
				defer body.Close()
				data, err := io.ReadAll(io.LimitReader(body, maxDiffSize+1))
				switch {
				case err != nil:
					return "", "", err
				case len(data) > maxDiffSize:
					return "", "", cli.Usagef("%s is too big to diff: use versions download", label)
				case textdiff.IsBinary(data):
					return "", "", cli.Usagef("%s is not text: use versions download to compare it", label)
				}
				return label, string(data), nil
			}
			oldLabel, oldText, err := read(keys[0])
			if err != nil {
				return err
			}
			newKey := ""
			if len(keys) == 2 {
				newKey = keys[1]
			}
			newLabel, newText, err := read(newKey)
			if err != nil {
				return err
			}
			diff := textdiff.Unified(oldText, newText, textdiff.Options{
				Context: context, OldLabel: oldLabel, NewLabel: newLabel,
				Colour: !plain && cli.UseColor(os.Stdout) && cmd.OutOrStdout() == os.Stdout,
			})
			if diff == "" {
				rt.Msg(cmd.ErrOrStderr(), "%s and %s are identical.", oldLabel, newLabel)
				return nil
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), diff)
			return err
		},
	}
	cmd.Flags().IntVarP(&context, "context", "U", 3, "unchanged lines to show around each change")
	cmd.Flags().BoolVar(&plain, "plain", false, "never color the output")
	return cmd
}
