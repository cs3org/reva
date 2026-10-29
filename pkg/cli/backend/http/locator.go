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
	"context"
	nethttp "net/http"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

type locator struct{ b *Backend }

func (l locator) AuthHeader(ctx context.Context) (nethttp.Header, error) {
	c, err := l.b.c.Credential(ctx)
	if err != nil {
		return nil, err
	}
	h := nethttp.Header{}
	if c.Header != "" {
		h.Set(c.Header, c.Value)
	}
	return h, nil
}

func (l locator) target(ctx context.Context, u string) (*backend.Target, error) {
	h, err := l.AuthHeader(ctx)
	if err != nil {
		return nil, err
	}
	return &backend.Target{URL: u, Header: h}, nil
}

func (l locator) DownloadTarget(ctx context.Context, ref *provider.Reference) (*backend.Target, error) {
	p, err := refPath(ref)
	if err != nil {
		return nil, err
	}
	u, err := l.b.c.DavURL(ctx, p)
	if err != nil {
		return nil, conv(err)
	}
	return l.target(ctx, u)
}

func (l locator) UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*backend.Target, error) {
	t, err := l.DownloadTarget(ctx, ref)
	if err != nil {
		return nil, err
	}
	t.Header.Set("Content-Type", "application/octet-stream")
	if ifMatch != "" {
		t.Header.Set("If-Match", `"`+strings.Trim(ifMatch, `"`)+`"`)
	}
	t.ChecksumHeader = true
	if caps, err := l.Capabilities(ctx); err == nil {
		t.Checksums = []*provider.ResourceChecksumPriority{}
		for i, name := range caps.Checksums {
			t.Checksums = append(t.Checksums, &provider.ResourceChecksumPriority{Type: storageprovider.PKG2GRPCXS(name), Priority: uint32(i)})
		}
	}
	return t, nil
}

func (l locator) ResumableTarget(ctx context.Context, ref *provider.Reference, size int64, checksum string) (*backend.Target, error) {
	p, err := refPath(ref)
	if err != nil {
		return nil, err
	}
	u, err := l.b.c.CreateUpload(ctx, p, size, checksum)
	if err != nil {
		return nil, conv(err)
	}
	return l.target(ctx, u)
}

func (l locator) VersionTarget(ctx context.Context, ref *provider.Reference, key string) (*backend.Target, error) {
	id, err := l.b.fileID(ctx, ref)
	if err != nil {
		return nil, err
	}
	return l.target(ctx, l.b.c.VersionURL(id, key))
}

func (l locator) ArchiveTarget(ctx context.Context, refs []*provider.Reference, format string) (*backend.Target, error) {
	paths := make([]string, len(refs))
	for i, ref := range refs {
		p, err := refPath(ref)
		if err != nil {
			return nil, err
		}
		paths[i] = p
	}
	u, err := l.b.c.ArchiveURL(ctx, paths, format)
	if err != nil {
		return nil, conv(err)
	}
	return l.target(ctx, u)
}

func (l locator) Capabilities(ctx context.Context) (*backend.Capabilities, error) {
	c, err := l.b.c.Capabilities(ctx)
	if err != nil {
		return nil, conv(err)
	}
	caps := &backend.Capabilities{
		TUS:           c.TusSupported,
		MaxChunkSize:  c.TusMaxChunkSize,
		Versioning:    c.Versioning,
		ServerVersion: c.ServerVersion,
		ServerProduct: c.ProductName,
	}
	best := c.BestChecksum()
	if best != "" {
		caps.Checksums = append(caps.Checksums, strings.ToLower(best))
	}
	for _, t := range c.ChecksumTypes {
		if !strings.EqualFold(t, best) {
			caps.Checksums = append(caps.Checksums, strings.ToLower(t))
		}
	}
	if c.ArchiverEnabled {
		caps.ArchiveFormats, caps.ArchiveMaxFiles, caps.ArchiveMaxSize = c.ArchiverFormats, c.ArchiverMaxNumFiles, c.ArchiverMaxSize
	}
	return caps, nil
}

// viewModes are the words that /app/open parses. It answers 401 for any other
// word.
var viewModes = map[gateway.OpenInAppRequest_ViewMode]string{
	gateway.OpenInAppRequest_VIEW_MODE_VIEW_ONLY:  "view",
	gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY:  "read",
	gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE: "write",
	gateway.OpenInAppRequest_VIEW_MODE_PREVIEW:    "preview",
	gateway.OpenInAppRequest_VIEW_MODE_EMBEDDED:   "embedded",
}

// OpenInApp uses the app service of the endpoint. Insecure and SkipVerify
// have no form there.
func (l locator) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	id, err := l.b.fileID(ctx, ref)
	if err != nil {
		return nil, err
	}
	s, err := l.b.c.OpenInApp(ctx, id, opts.App, viewModes[opts.ViewMode])
	if err != nil {
		return nil, conv(err)
	}
	return &appprovider.OpenInAppURL{AppUrl: s.URL, Method: s.Method, FormParameters: s.FormParameters, Headers: s.Headers}, nil
}
