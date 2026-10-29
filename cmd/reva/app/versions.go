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
	"context"
	"fmt"
	"io"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/spf13/cobra"
)

func versionsCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "versions", Short: "Manage the versions of a file"}
	cmd.AddCommand(&cobra.Command{
		Use:               "list PATH",
		Short:             "List the versions of a file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			vs, err := b.Versions().List(cmd.Context(), pathRef(args[0]))
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), vs, func(w io.Writer) {
				for _, v := range vs {
					fmt.Fprintf(w, "Key: %s Size: %d mtime:%d\n", v.Key, v.Size, v.Mtime)
				}
			})
		},
	}, versionsRestoreCommand(rt), versionsDownloadCommand(rt), versionsDiffCommand(rt))
	return cmd
}

func trashCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trash",
		Short: "Work with deleted files",
		Long:  "Work with the deleted files of your home space.",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List deleted files",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				b, err := rt.Backend(cmd.Context())
				if err != nil {
					return err
				}
				items, err := b.Trash().List(cmd.Context(), nil, time.Time{}, time.Time{})
				if err != nil {
					return err
				}
				return rt.Print(cmd.OutOrStdout(), items, func(w io.Writer) {
					for _, item := range items {
						fmt.Fprintf(w, "%+v\n", item)
					}
				})
			},
		},
		&cobra.Command{
			Use:   "restore KEY...",
			Short: "Restore deleted files",
			Long: "Restore deleted files. They go back where they were, which is the path\n" +
				"shown by 'reva trash list'.",
			Args: cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				b, err := rt.Backend(cmd.Context())
				if err != nil {
					return err
				}
				for _, key := range args {
					if err := b.Trash().Restore(cmd.Context(), nil, key); err != nil {
						return err
					}
					rt.Msg(cmd.ErrOrStderr(), "Restored %s", key)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "purge KEY...",
			Short: "Delete items from the trash for good",
			Long:  "Delete trash items for good. This cannot be undone.",
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				b, err := rt.Backend(cmd.Context())
				if err != nil {
					return err
				}
				for _, key := range args {
					if err := b.Trash().Purge(cmd.Context(), nil, key); err != nil {
						return err
					}
					rt.Msg(cmd.ErrOrStderr(), "Purged %s", key)
				}
				return nil
			},
		},
	)
	return cmd
}

func lockCommand(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "lock", Short: "Manage the lock of a file"}
	cmd.AddCommand(lockGetCommand(rt), lockSetCommand(rt), lockRemoveCommand(rt))
	return cmd
}

func lockGetCommand(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:               "get PATH",
		Short:             "Show the lock of a file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := rt.Backend(cmd.Context())
			if err != nil {
				return err
			}
			lock, err := b.Locks().Get(cmd.Context(), pathRef(args[0]))
			if err != nil {
				return err
			}
			return rt.Print(cmd.OutOrStdout(), lock, func(w io.Writer) { fmt.Fprintln(w, lock) })
		},
	}
}

func lockSetCommand(rt *cli.Runtime) *cobra.Command {
	var lockType, id, username, app, exp string
	var refresh bool
	cmd := &cobra.Command{
		Use:               "set PATH",
		Short:             "Set or refresh the lock of a file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			lock, err := createLock(ctx, b, lockType, id, username, app, exp)
			if err != nil {
				return err
			}
			set := b.Locks().Set
			if refresh {
				set = b.Locks().Refresh
			}
			if err := set(ctx, pathRef(args[0]), lock); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&lockType, "type", "write", "type of lock: shared, write or exclusive")
	cmd.Flags().StringVar(&id, "id", "", "id of the lock")
	cmd.Flags().StringVar(&username, "user", "", "user that holds the lock")
	cmd.Flags().StringVar(&app, "app", "", "app that holds the lock")
	cmd.Flags().StringVar(&exp, "exp", "", "expiration date of the lock (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh an existing lock")
	return cmd
}

func lockRemoveCommand(rt *cli.Runtime) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:               "remove PATH",
		Short:             "Remove the lock of a file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRemotePath(rt, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			b, err := rt.Backend(ctx)
			if err != nil {
				return err
			}
			ref := pathRef(args[0])
			lock := &provider.Lock{LockId: id}
			if id == "" {
				if lock, err = b.Locks().Get(ctx, ref); err != nil {
					return err
				}
			}
			if err := b.Locks().Unlock(ctx, ref, lock); err != nil {
				return err
			}
			rt.Msg(cmd.ErrOrStderr(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "id of the lock (default: the current lock)")
	return cmd
}

func createLock(ctx context.Context, b backend.Backend, t, id, u, app, exp string) (*provider.Lock, error) {
	lock := &provider.Lock{LockId: id, AppName: app}
	switch t {
	case "shared":
		lock.Type = provider.LockType_LOCK_TYPE_SHARED
	case "write":
		lock.Type = provider.LockType_LOCK_TYPE_WRITE
	case "exclusive":
		lock.Type = provider.LockType_LOCK_TYPE_EXCL
	default:
		return nil, fmt.Errorf("unknown lock type %q: use shared, write or exclusive", t)
	}
	if u != "" {
		user, err := b.Users().ByClaim(ctx, "username", u)
		if err != nil {
			return nil, err
		}
		lock.User = user.GetId()
	}
	if exp != "" {
		t, err := time.Parse(time.DateOnly, exp)
		if err != nil {
			return nil, err
		}
		lock.Expiration = &types.Timestamp{Seconds: uint64(t.Unix())}
	}
	return lock, nil
}
