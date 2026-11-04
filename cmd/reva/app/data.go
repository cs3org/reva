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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cheggaaa/pb"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/spf13/cobra"
)

// The prefixes of a cp argument. A path is local unless it starts with cb:,
// because an absolute path like /eos/... can exist on the local machine too.
// file: marks a local path whose name starts with cb:.
const (
	remotePrefix = "cb:"
	localPrefix  = "file:"
)

func cpCommand(rt *cli.Runtime) *cobra.Command {
	var force bool
	var xsFlag, protocol string
	cmd := &cobra.Command{
		Use:   "cp SOURCE DEST",
		Short: "Copy between your computer and reva",
		Long: "Copy a file between your computer and reva.\n\n" +
			"Mark the remote side with cb:, because a path like /eos/... can exist on your\n" +
			"computer too. A destination that is a folder, or that ends with /, gets the\n" +
			"name of the source. An existing destination is skipped unless you pass -f.",
		Example: "  reva cp ./report.pdf cb:/home/Documents/\n" +
			"  reva cp cb:/home/Documents/report.pdf ./report.pdf",
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
			src, srcRemote := splitTransferArg(args[0])
			dst, dstRemote := splitTransferArg(args[1])
			switch {
			case srcRemote && dstRemote:
				if err := rt.ConfirmMutation(cmd.CommandPath()); err != nil {
					return err
				}
				b, err := rt.Backend(cmd.Context())
				if err != nil {
					return err
				}
				return b.Files().Copy(cmd.Context(), pathRef(src), pathRef(dst), force)
			case !srcRemote && !dstRemote:
				return fmt.Errorf("both %q and %q are local paths: mark the remote side with %q, for example %s%s",
					args[0], args[1], remotePrefix, remotePrefix, args[1])
			case dstRemote:
				if err := rt.ConfirmMutation(cmd.CommandPath()); err != nil {
					return err
				}
				return upload(cmd, rt, src, dst, force, xsFlag, protocol)
			}
			return download(cmd, rt, src, dst, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite existing destinations")
	cmd.Flags().StringVar(&xsFlag, "xs", "negotiate", "checksum type of an upload, or negotiate to use the server preference")
	cmd.Flags().StringVar(&protocol, "protocol", "simple", "upload protocol")
	return cmd
}

func splitTransferArg(arg string) (string, bool) {
	if rest, ok := strings.CutPrefix(arg, localPrefix); ok {
		return rest, false
	}
	if rest, ok := strings.CutPrefix(arg, remotePrefix); ok {
		return rest, true
	}
	return arg, false
}

func upload(cmd *cobra.Command, rt *cli.Runtime, local, target string, force bool, xsFlag, protocol string) error {
	if protocol != "simple" {
		return errors.New("upload: protocol not supported: " + protocol)
	}
	absPath, err := utils.ResolvePath(local)
	if err != nil {
		return err
	}
	md, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if md.IsDir() {
		return fmt.Errorf("%s is a folder: cp copies only files for now", local)
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
	if err := d.Upload(ctx, pathRef(target), open, md.Size(), xsFlag); err != nil {
		return err
	}
	rt.Msg(cmd.ErrOrStderr(), "Uploaded %s to %s (%d bytes)", local, target, md.Size())
	return nil
}

func download(cmd *cobra.Command, rt *cli.Runtime, remote, local string, force bool) error {
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
		return fmt.Errorf("%s is a folder: cp copies only files for now", remote)
	}

	absPath, err := utils.ResolvePath(local)
	if err != nil {
		return err
	}
	if st, err := os.Stat(absPath); err == nil && st.IsDir() {
		absPath = filepath.Join(absPath, path.Base(info.Path))
	}
	if _, err := os.Stat(absPath); err == nil && !force {
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
