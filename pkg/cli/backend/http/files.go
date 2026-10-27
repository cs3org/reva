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

package http

import (
	"cmp"
	"context"
	"errors"
	"path"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/backend/http/internal/client"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

type files struct{ b *Backend }

// refPath returns the absolute path of ref. WebDAV addresses files by path,
// so a reference by resource id is not available.
func refPath(ref *provider.Reference) (string, error) {
	p := ref.GetPath()
	if ref.GetResourceId() != nil || !strings.HasPrefix(p, "/") {
		return "", notAvailable("a reference by resource id")
	}
	return p, nil
}

func (f files) Stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	p, err := refPath(ref)
	if err != nil {
		return nil, err
	}
	info, err := f.b.c.Stat(ctx, p)
	if err != nil {
		return nil, conv(err)
	}
	return resourceInfo(info), nil
}

func (f files) List(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	p, err := refPath(ref)
	if err != nil {
		return nil, err
	}
	infos, err := f.b.c.List(ctx, p)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*provider.ResourceInfo, len(infos))
	for i := range infos {
		out[i] = resourceInfo(&infos[i])
	}
	return out, nil
}

// fnError keeps the errors of the callback of Walk apart from those of the
// client, so that conv does not change them.
type fnError struct{ err error }

func (e fnError) Error() string { return e.err.Error() }

func (f files) Walk(ctx context.Context, ref *provider.Reference, fn func(*provider.ResourceInfo) error) error {
	p, err := refPath(ref)
	if err != nil {
		return err
	}
	err = f.b.c.Walk(ctx, p, func(i client.ResourceInfo) error {
		if err := fn(resourceInfo(&i)); err != nil {
			return fnError{err}
		}
		return nil
	})
	if fe, ok := errors.AsType[fnError](err); ok {
		return fe.err
	}
	return conv(err)
}

func (f files) Mkdir(ctx context.Context, ref *provider.Reference, parents bool) error {
	p, err := refPath(ref)
	if err != nil {
		return err
	}
	return conv(f.b.c.Mkdir(ctx, p, parents))
}

func (f files) Move(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return f.moveOrCopy(ctx, src, dst, overwrite, f.b.c.Move)
}

func (f files) Copy(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return f.moveOrCopy(ctx, src, dst, overwrite, f.b.c.Copy)
}

func (f files) moveOrCopy(ctx context.Context, src, dst *provider.Reference, overwrite bool, do func(context.Context, string, string, bool) error) error {
	s, err := refPath(src)
	if err != nil {
		return err
	}
	d, err := refPath(dst)
	if err != nil {
		return err
	}
	err = do(ctx, s, d, overwrite)
	// WebDAV refuses an existing destination with 412 when Overwrite is F.
	if ce, ok := errors.AsType[*cberr.Error](err); ok && !overwrite && ce.Status == 412 {
		return &backend.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: d + " exists"}
	}
	return conv(err)
}

func (f files) Remove(ctx context.Context, ref *provider.Reference) error {
	p, err := refPath(ref)
	if err != nil {
		return err
	}
	return conv(f.b.c.Remove(ctx, p))
}

func (f files) Touch(ctx context.Context, ref *provider.Reference) error {
	p, err := refPath(ref)
	if err != nil {
		return err
	}
	return conv(f.b.c.Touch(ctx, p))
}

func (f files) ResolveSpace(ctx context.Context, alias string) (*provider.Reference, error) {
	root, err := f.b.c.ResolveSpace(ctx, alias)
	if err != nil {
		return nil, conv(err)
	}
	return &provider.Reference{Path: root}, nil
}

func (b *Backend) Search(ctx context.Context, ref *provider.Reference, opts backend.SearchOptions) ([]*provider.ResourceInfo, error) {
	p, err := refPath(ref)
	if err != nil {
		return nil, err
	}
	infos, err := b.c.Search(ctx, p, client.SearchOptions{Pattern: opts.Pattern, Limit: opts.Limit})
	if errors.Is(err, client.ErrSearchUnsupported) {
		return nil, notAvailable("search")
	}
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*provider.ResourceInfo, len(infos))
	for i := range infos {
		out[i] = resourceInfo(&infos[i])
	}
	return out, nil
}

// resourceInfo converts a PROPFIND entry. The owner has no source, and the
// permissions are those of the role of oc:permissions, not the exact set.
func resourceInfo(i *client.ResourceInfo) *provider.ResourceInfo {
	ri := &provider.ResourceInfo{
		Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
		Path:          i.Path,
		Name:          cmp.Or(i.Name, path.Base(i.Path)),
		Size:          uint64(max(i.Size, 0)),
		Etag:          i.ETag,
		MimeType:      i.MimeType,
		Mtime:         timestamp(i.Modified),
		PermissionSet: davPermissions(i.Permissions),
	}
	if i.IsDir {
		ri.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	if id, ok := spaces.ParseResourceID(i.ID); ok {
		ri.Id = id
	}
	for _, name := range []string{"adler32", "md5", "sha1"} {
		if sum := i.Checksums[name]; sum != "" {
			ri.Checksum = &provider.ResourceChecksum{Type: storageprovider.PKG2GRPCXS(name), Sum: sum}
			break
		}
	}
	return ri
}

// davPermissions reads the letters of oc:permissions (pkg/permissions,
// WebDAVPermissions). They say what the role of the caller allows, so the
// set is not exact. An empty string is unknown, and gives nil.
func davPermissions(s string) *provider.ResourcePermissions {
	if s == "" {
		return nil
	}
	p := &provider.ResourcePermissions{
		Stat: true, GetPath: true, ListContainer: true, InitiateFileDownload: true,
		ListFileVersions: true, ListRecycle: true, GetQuota: true,
	}
	for _, l := range s {
		switch l {
		case 'R':
			p.AddGrant, p.ListGrants, p.UpdateGrant, p.RemoveGrant = true, true, true, true
		case 'D':
			p.Delete, p.PurgeRecycle, p.RestoreRecycleItem = true, true, true
		case 'N', 'V', 'W':
			p.InitiateFileUpload, p.Move, p.RestoreFileVersion = true, true, true
		case 'C', 'K':
			p.InitiateFileUpload, p.CreateContainer = true, true
		case 'Z':
			p.DenyGrant = true
		}
	}
	return p
}

func timestamp(t time.Time) *types.Timestamp {
	if t.IsZero() {
		return nil
	}
	return &types.Timestamp{Seconds: uint64(t.Unix()), Nanos: uint32(t.Nanosecond())}
}
