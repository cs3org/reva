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

package main

import (
	"fmt"
	"io"
	"path"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/spf13/cobra"
)

func lsCommand(rt *cli.Runtime) *cobra.Command {
	var long, full bool
	cmd := &cobra.Command{
		Use:               "ls PATH",
		Short:             "List the contents of a folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ListContainer(ctx, &provider.ListContainerRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Infos, func(w io.Writer) {
				for _, info := range res.Infos {
					p := info.Path
					if !full {
						p = path.Base(info.Path)
					}
					if long {
						fmt.Fprintf(w, "%s %s %d %v %s\n", info.Type, info.Mtime, info.Size, info.Id, p)
					} else {
						fmt.Fprintln(w, p)
					}
				}
			})
		},
	}
	cmd.Flags().BoolVarP(&long, "long", "l", false, "long listing")
	cmd.Flags().BoolVarP(&full, "full", "f", false, "show the full path")
	return cmd
}

func statCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "stat PATH",
		Short:             "Show the metadata of a file or folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return rt.Print(cmd.OutOrStdout(), res.Info, func(w io.Writer) { fmt.Fprintln(w, res.Info) })
		},
	}
}

func mkdirCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "mkdir PATH",
		Short:             "Create a folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.CreateContainer(ctx, &provider.CreateContainerRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return nil
		},
	}
}

func mvCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "mv SOURCE DEST",
		Short:             "Move or rename a file or folder",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeRemotePath(rt, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Move(ctx, &provider.MoveRequest{
				Source:      &provider.Reference{Path: args[0]},
				Destination: &provider.Reference{Path: args[1]},
			})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return nil
		},
	}
}

func rmCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "rm PATH",
		Short:             "Remove a file or folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.Delete(ctx, &provider.DeleteRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return nil
		},
	}
}

func touchCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "touch PATH",
		Short:             "Create an empty file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Gateway(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.TouchFile(ctx, &provider.TouchFileRequest{Ref: &provider.Reference{Path: args[0]}})
			if err != nil {
				return err
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
			return nil
		},
	}
}

// completeRemotePath completes the first n arguments as remote paths, by
// listing the folder of the word under the cursor.
func completeRemotePath(rt *cli.Runtime, n int) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) >= n {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return listRemote(cmd, rt, toComplete)
	}
}

func listRemote(cmd *cobra.Command, rt *cli.Runtime, toComplete string) ([]string, cobra.ShellCompDirective) {
	none := cobra.ShellCompDirectiveNoFileComp
	if !strings.HasPrefix(toComplete, "/") {
		toComplete = "/"
	}
	dir := toComplete
	if !strings.HasSuffix(dir, "/") {
		dir = path.Dir(dir)
	}
	// Completion does not run the PersistentPreRunE, so load the settings here.
	if err := rt.Load(cmd.Flags()); err != nil {
		return nil, none
	}
	client, ctx, err := rt.Gateway(cmd.Context())
	if err != nil {
		return nil, none
	}
	res, err := client.ListContainer(ctx, &provider.ListContainerRequest{Ref: &provider.Reference{Path: dir}})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		return nil, none
	}
	out := make([]string, 0, len(res.Infos))
	for _, info := range res.Infos {
		p := path.Join(dir, path.Base(info.Path))
		if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			p += "/"
		}
		out = append(out, p)
	}
	return out, none | cobra.ShellCompDirectiveNoSpace
}
