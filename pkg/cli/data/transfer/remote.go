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

package transfer

import (
	"context"
	"io"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// ResourceInfo describes a remote file.
type ResourceInfo struct {
	Path     string
	IsDir    bool
	Size     int64
	Modified time.Time
	ETag     string
	// Checksums are by type in lower case, for example adler32.
	Checksums map[string]string
}

type remote struct {
	f backend.Files
	d backend.DataPlane
}

func ref(p string) *provider.Reference { return &provider.Reference{Path: p} }

func info(ri *provider.ResourceInfo) ResourceInfo {
	i := ResourceInfo{
		Path:  ri.GetPath(),
		IsDir: ri.GetType() == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Size:  int64(ri.GetSize()),
		ETag:  ri.GetEtag(),
	}
	if m := ri.GetMtime(); m != nil {
		i.Modified = time.Unix(int64(m.Seconds), int64(m.Nanos))
	}
	if xs := ri.GetChecksum(); xs.GetSum() != "" {
		i.Checksums = map[string]string{strings.ToLower(storageprovider.GRPC2PKGXS(xs.Type).String()): xs.Sum}
	}
	return i
}

func (r remote) Stat(ctx context.Context, p string) (*ResourceInfo, error) {
	ri, err := r.f.Stat(ctx, ref(p))
	if err != nil {
		return nil, err
	}
	i := info(ri)
	return &i, nil
}

func (r remote) Walk(ctx context.Context, p string, fn func(ResourceInfo) error) error {
	return r.f.Walk(ctx, ref(p), func(ri *provider.ResourceInfo) error { return fn(info(ri)) })
}

func (r remote) Mkdir(ctx context.Context, p string, parents bool) error {
	return r.f.Mkdir(ctx, ref(p), parents)
}

func (r remote) Remove(ctx context.Context, p string) error { return r.f.Remove(ctx, ref(p)) }

func (r remote) Upload(ctx context.Context, p string, open func() (io.ReadCloser, error), size int64, checksum string) error {
	return r.d.Upload(ctx, ref(p), open, size, checksum)
}

func (r remote) Download(ctx context.Context, p string, offset int64) (io.ReadCloser, int64, error) {
	return r.d.Download(ctx, ref(p), offset)
}

// caps returns empty capabilities when the server gives none: no TUS, no archiver.
func (r remote) caps(ctx context.Context) *backend.Capabilities {
	c, err := r.d.Capabilities(ctx)
	if err != nil || c == nil {
		return &backend.Capabilities{}
	}
	return c
}
