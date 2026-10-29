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
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/data/transfer"
	"github.com/cs3org/reva/v3/pkg/cli/output"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/spf13/cobra"
)

const remotePrefix = "cb:"

type transferFlags struct {
	recursive, force, dryRun, verify, noArchive bool
	jobs                                        int
}

func (t *transferFlags) register(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVarP(&t.recursive, "recursive", "r", false, "copy folders and their contents")
	f.BoolVarP(&t.recursive, "recursive-upper", "R", false, "same as --recursive")
	_ = f.MarkHidden("recursive-upper")
	f.BoolVarP(&t.force, "force", "f", false, "overwrite existing destinations")
	f.BoolVar(&t.dryRun, "dry-run", false, "report what would be transferred without doing it")
	f.BoolVar(&t.verify, "verify", false, "compute and check checksums")
	f.BoolVar(&t.noArchive, "no-archive", false, "download a folder file by file instead of as one archive")
	f.IntVarP(&t.jobs, "jobs", "j", 0, "number of files to transfer at once")
}

func (t *transferFlags) engine(cmd *cobra.Command, rt *cli.Runtime) (*transfer.Engine, error) {
	var progress transfer.ProgressFunc
	if rt.ShowProgress() {
		progress = func(ev transfer.Event) {
			switch {
			case !ev.Done:
			case ev.Err != nil:
				fmt.Fprintf(cmd.ErrOrStderr(), "  ! %s: %v\n", ev.Path, ev.Err)
			default:
				fmt.Fprintf(cmd.ErrOrStderr(), "  %s (%s)\n", ev.Path, output.HumanSize(ev.Transferred))
			}
		}
	}
	return rt.Engine(cmd.Context(), cli.TransferOptions{
		DryRun: t.dryRun, Overwrite: t.force, Verify: t.verify, NoArchive: t.noArchive, Jobs: t.jobs, Progress: progress,
	})
}

func reportTransfer(cmd *cobra.Command, rt *cli.Runtime, stats *transfer.Stats, verb string) error {
	rt.Msg(cmd.ErrOrStderr(), "%s %d files (%s) in %s, %d skipped", verb, stats.Files, output.HumanSize(stats.Bytes),
		stats.Duration.Round(time.Millisecond), stats.Skipped)
	if rt.Output() == "json" {
		return rt.Print(cmd.OutOrStdout(), stats, nil)
	}
	return nil
}

func cpCommand(rt *cli.Runtime) *cobra.Command {
	var flags transferFlags
	var xsFlag, protocol string
	cmd := &cobra.Command{
		Use:   "cp SOURCE DEST",
		Short: "Copy between your computer and reva",
		Long: "Copy files between your computer and reva.\n\n" +
			"Mark the remote side with cb:, because a path like /eos/... can exist on your\n" +
			"computer too, or write it with a space alias (home:Documents). A destination\n" +
			"that is a folder, or that ends with /, gets the name of the source. An\n" +
			"existing destination is skipped unless you pass -f.\n\n" +
			"A folder needs -r. Large uploads go up in parts, and an interrupted transfer\n" +
			"carries on where it stopped. A folder comes down as one archive when the\n" +
			"server has an archiver.",
		Example: "  reva cp ./report.pdf cb:/home/Documents/\n" +
			"  reva cp cb:/home/Documents/report.pdf ./report.pdf\n" +
			"  reva cp -r ./data home:data",
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) >= 2 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			rest, ok := strings.CutPrefix(toComplete, remotePrefix)
			if !ok {
				return nil, cobra.ShellCompDirectiveDefault
			}
			paths, directive := listRemote(cmd, rt, rest)
			for i := range paths {
				paths[i] = remotePrefix + paths[i]
			}
			return paths, directive
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			src, dst, srcRemote, dstRemote, intoDir, err := rt.TransferPaths(ctx, args[0], args[1])
			if err != nil {
				return err
			}
			if intoDir {
				dst += "/"
			}
			switch {
			case srcRemote && dstRemote:
				if err := rt.ConfirmMutation(cmd.CommandPath()); err != nil {
					return err
				}
				b, err := rt.Backend(ctx)
				if err != nil {
					return err
				}
				if info, err := b.Files().Stat(ctx, pathRef(dst)); intoDir || err == nil && info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					dst = path.Join(dst, path.Base(src))
				}
				if flags.dryRun {
					rt.Msg(cmd.ErrOrStderr(), "Would copy %s to %s", src, dst)
					return nil
				}
				return b.Files().Copy(ctx, pathRef(src), pathRef(path.Clean(dst)), flags.force)
			case dstRemote:
				if err := rt.ConfirmMutation(cmd.CommandPath()); err != nil {
					return err
				}
				if st, err := os.Stat(src); err == nil && st.IsDir() {
					return uploadTree(cmd, rt, &flags, src, dst)
				}
				return upload(cmd, rt, src, dst, flags.force, xsFlag, protocol)
			}
			return download(cmd, rt, &flags, src, dst)
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&xsFlag, "xs", "negotiate", "checksum type of an upload, or negotiate to use the server preference")
	cmd.Flags().StringVar(&protocol, "protocol", "simple", "upload protocol of a file: simple or tus (resumable)")
	return cmd
}

func uploadTree(cmd *cobra.Command, rt *cli.Runtime, flags *transferFlags, local, remote string) error {
	if !flags.recursive {
		return cli.Usagef("%s is a folder: pass -r to upload it", local)
	}
	b, err := rt.Backend(cmd.Context())
	if err != nil {
		return err
	}
	if info, err := b.Files().Stat(cmd.Context(), pathRef(remote)); strings.HasSuffix(remote, "/") ||
		err == nil && info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		remote = path.Join(remote, filepath.Base(filepath.Clean(local)))
	}
	e, err := flags.engine(cmd, rt)
	if err != nil {
		return err
	}
	stats, err := e.UploadTree(cmd.Context(), local, path.Clean(remote))
	if err != nil {
		return err
	}
	return reportTransfer(cmd, rt, stats, "Uploaded")
}

func upload(cmd *cobra.Command, rt *cli.Runtime, local, target string, force bool, xsFlag, protocol string) error {
	if protocol != "simple" && protocol != "tus" {
		return cli.Usagef("upload: protocol not supported: %s: use simple or tus", protocol)
	}
	absPath, err := utils.ResolvePath(local)
	if err != nil {
		return err
	}
	md, err := os.Stat(absPath)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	b, err := rt.Backend(ctx)
	if err != nil {
		return err
	}
	d, err := rt.Data(ctx)
	if err != nil {
		return err
	}
	// The cp(1) rule: a destination that is a folder means "into it".
	info, err := b.Files().Stat(ctx, pathRef(target))
	if err != nil && !cli.IsNotFound(err) {
		return err
	}
	if err == nil && info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER || strings.HasSuffix(target, "/") {
		target = path.Join(target, filepath.Base(absPath))
		if _, err = b.Files().Stat(ctx, pathRef(target)); err != nil && !cli.IsNotFound(err) {
			return err
		}
	}
	if err == nil && !force {
		rt.Msg(cmd.ErrOrStderr(), "Skipped %s: it exists, pass -f to overwrite it", target)
		return nil
	}
	rt.Debugf("local file size: %d bytes", md.Size())

	open := func() (io.ReadCloser, error) { return os.Open(absPath) }
	if protocol == "tus" {
		err = d.UploadResumable(ctx, pathRef(target), open, md.Size(), "", nil)
	} else {
		err = d.Upload(ctx, pathRef(target), open, md.Size(), xsFlag)
	}
	if err != nil {
		return err
	}
	rt.Msg(cmd.ErrOrStderr(), "Uploaded %s to %s (%d bytes)", local, target, md.Size())
	return nil
}

func download(cmd *cobra.Command, rt *cli.Runtime, flags *transferFlags, remote, local string) error {
	ctx := cmd.Context()
	b, err := rt.Backend(ctx)
	if err != nil {
		return err
	}
	d, err := rt.Data(ctx)
	if err != nil {
		return err
	}
	info, err := b.Files().Stat(ctx, pathRef(remote))
	if err != nil {
		return err
	}
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if !flags.recursive {
			return cli.Usagef("%s is a folder: pass -r to download it", remote)
		}
		if st, err := os.Stat(local); err == nil && st.IsDir() {
			local = filepath.Join(local, path.Base(remote))
		}
		e, err := flags.engine(cmd, rt)
		if err != nil {
			return err
		}
		stats, err := e.DownloadTree(ctx, remote, local)
		if err != nil {
			return err
		}
		return reportTransfer(cmd, rt, stats, "Downloaded")
	}

	absPath, err := utils.ResolvePath(local)
	if err != nil {
		return err
	}
	if st, err := os.Stat(absPath); err == nil && st.IsDir() {
		absPath = filepath.Join(absPath, path.Base(info.Path))
	}
	if _, err := os.Stat(absPath); err == nil && !flags.force {
		rt.Msg(cmd.ErrOrStderr(), "Skipped %s: it exists, pass -f to overwrite it", absPath)
		return nil
	}

	body, _, err := d.Download(ctx, pathRef(remote), 0)
	if err != nil {
		return err
	}
	defer body.Close()
	var content io.Reader = body

	fd, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	var bar *pb.ProgressBar
	if rt.ShowProgress() {
		bar = pb.New(int(info.Size)).SetUnits(pb.U_BYTES)
		bar.Output = os.Stderr
		bar.Start()
		content = bar.NewProxyReader(content)
	}
	n, err := io.Copy(fd, content)
	if bar != nil {
		bar.Finish()
	}
	if err != nil {
		fd.Close()
		return err
	}
	if err := fd.Close(); err != nil {
		return err
	}
	rt.Msg(cmd.ErrOrStderr(), "Downloaded %s to %s (%d bytes)", remote, absPath, n)
	return nil
}
