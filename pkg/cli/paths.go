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

package cli

import (
	"cmp"
	"context"
	"path"

	"github.com/cs3org/reva/v3"
	"github.com/cs3org/reva/v3/internal/cli/pathspec"
	"github.com/cs3org/reva/v3/pkg/cli/data/transfer"
)

// RemotePath resolves a remote path argument: by a cli.paths.* plugin
// first, else an absolute path, a space alias with a path in it
// (home:Documents), or a path relative to the home space.
func (r *Runtime) RemotePath(ctx context.Context, arg string) (string, error) {
	for _, p := range reva.GetPlugins(PathsNamespace) {
		newFn, ok := p.New.(func(*Runtime) PathResolver)
		if !ok {
			continue
		}
		if p, ok, err := newFn(r).Resolve(ctx, arg); ok || err != nil {
			return p, err
		}
	}
	spec, err := pathspec.ParseRemote(arg)
	if err != nil {
		return "", &UsageError{Err: err}
	}
	return r.resolveSpec(ctx, spec)
}

// TransferPaths splits the two arguments of a transfer: the remote side has
// cb: or a space alias. It returns the resolved remote paths.
func (r *Runtime) TransferPaths(ctx context.Context, src, dst string) (from, to string, fromRemote, toRemote, intoDir bool, err error) {
	s, d, err := pathspec.ParseTransferPair(src, dst)
	if err != nil {
		return "", "", false, false, false, &UsageError{Err: err}
	}
	from, to = s.Path, d.Path
	if s.IsRemote() {
		if from, err = r.resolveSpec(ctx, s); err != nil {
			return
		}
	}
	if d.IsRemote() {
		if to, err = r.resolveSpec(ctx, d); err != nil {
			return
		}
	}
	return from, to, s.IsRemote(), d.IsRemote(), d.TrailingSlash, nil
}

func (r *Runtime) resolveSpec(ctx context.Context, spec pathspec.Spec) (string, error) {
	if spec.Space == "" {
		return spec.Path, nil
	}
	b, err := r.Backend(ctx)
	if err != nil {
		return "", err
	}
	root, err := b.Files().ResolveSpace(ctx, spec.Space)
	if err != nil {
		return "", err
	}
	if root.GetPath() == "" {
		return "", Usagef("the space %s has no path: use a path that starts with /", spec.Space)
	}
	return path.Join(root.GetPath(), spec.Path), nil
}

// TransferOptions are the options of a transfer command.
type TransferOptions struct {
	DryRun, Overwrite, Verify, NoArchive bool
	Jobs                                 int
	Progress                             transfer.ProgressFunc
}

// Engine returns the transfer engine on the user plane, with the transfer
// settings of the config and of the brand.
func (r *Runtime) Engine(ctx context.Context, o TransferOptions) (*transfer.Engine, error) {
	b, err := r.Backend(ctx)
	if err != nil {
		return nil, err
	}
	d, err := r.Data(ctx)
	if err != nil {
		return nil, err
	}
	t := r.Transfer()
	return transfer.New(b.Files(), d, transfer.Options{
		Jobs:      cmp.Or(o.Jobs, t.Jobs),
		ChunkSize: t.ChunkSize,
		DryRun:    o.DryRun,
		Overwrite: o.Overwrite,
		Verify:    o.Verify,
		Archive:   t.Archive && !o.NoArchive,
		StateDir:  transfer.DefaultStateDir(r.Brand().ConfigDir()),
		Progress:  o.Progress,
	}), nil
}
