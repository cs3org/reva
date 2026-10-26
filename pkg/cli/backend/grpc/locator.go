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

package grpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

const (
	simpleProtocol = "simple"
	tusProtocol    = "tus"
)

type locator struct{ b *Backend }

func (l locator) DownloadTarget(ctx context.Context, ref *provider.Reference) (*backend.Target, error) {
	return l.download(ctx, &provider.InitiateFileDownloadRequest{Ref: ref})
}

func (l locator) VersionTarget(ctx context.Context, ref *provider.Reference, key string) (*backend.Target, error) {
	return l.download(ctx, &provider.InitiateFileDownloadRequest{Ref: ref, Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
		"version_key": {Decoder: "plain", Value: []byte(key)},
	}}})
}

func (l locator) download(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*backend.Target, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.InitiateFileDownload(ctx, req))
	if err != nil {
		return nil, err
	}
	for _, p := range res.Protocols {
		if p.Protocol == simpleProtocol {
			t, _, err := target(ctx, p.DownloadEndpoint, p.Token, p.Opaque)
			return t, err
		}
	}
	return nil, fmt.Errorf("download: the server offers no %s protocol", simpleProtocol)
}

func (l locator) UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*backend.Target, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	req := &provider.InitiateFileUploadRequest{
		Ref: ref,
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
			"Upload-Length": {Decoder: "plain", Value: []byte(strconv.FormatInt(size, 10))},
		}},
	}
	if ifMatch != "" {
		req.Options = &provider.InitiateFileUploadRequest_IfMatch{IfMatch: ifMatch}
	}
	res, err := check(c.InitiateFileUpload(ctx, req))
	if err != nil {
		return nil, err
	}
	for _, p := range res.Protocols {
		if p.Protocol != simpleProtocol {
			continue
		}
		t, dav, err := target(ctx, p.UploadEndpoint, p.Token, p.Opaque)
		if err != nil {
			return nil, err
		}
		t.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
		if !dav {
			// Not nil even when empty: the data gateway takes a checksum, but
			// the server offers no type to negotiate.
			t.Checksums = append([]*provider.ResourceChecksumPriority{}, p.AvailableChecksums...)
		}
		return t, nil
	}
	return nil, fmt.Errorf("upload: the server offers no %s protocol", simpleProtocol)
}

// ResumableTarget puts the transfer token in the path, as ocdav does,
// because a TUS client sends no header of its own (the data gateway reads
// the token from there).
func (l locator) ResumableTarget(ctx context.Context, ref *provider.Reference, size int64, _ string) (*backend.Target, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Ref: ref,
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
			"Upload-Length": {Decoder: "plain", Value: []byte(strconv.FormatInt(size, 10))},
		}},
	}))
	if err != nil {
		return nil, err
	}
	for _, p := range res.Protocols {
		if p.Protocol != tusProtocol {
			continue
		}
		u := p.UploadEndpoint
		if p.Token != "" {
			if u, err = url.JoinPath(u, p.Token); err != nil {
				return nil, err
			}
		}
		h, _ := l.AuthHeader(ctx)
		return &backend.Target{URL: u, Header: h}, nil
	}
	return nil, notAvailable("TUS uploads")
}

// ArchiveTarget: the archiver is an HTTP service that the gateway does not
// know.
func (l locator) ArchiveTarget(ctx context.Context, refs []*provider.Reference, format string) (*backend.Target, error) {
	return nil, notAvailable("archive")
}

// Capabilities has no CS3 call. The simple protocol always works, so the
// data plane does not try TUS.
func (l locator) Capabilities(ctx context.Context) (*backend.Capabilities, error) {
	return &backend.Capabilities{}, nil
}

func (l locator) AuthHeader(ctx context.Context) (http.Header, error) {
	_, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	if t, ok := appctx.ContextGetToken(ctx); ok {
		h.Set(appctx.TokenHeader, t)
	}
	return h, nil
}

// target is the data gateway with the transfer token, or the WebDAV path
// that some storage providers put in the opaque map instead.
func target(ctx context.Context, endpoint, token string, o *types.Opaque) (t *backend.Target, dav bool, err error) {
	davToken, davPath, dav, err := webdavRef(o)
	if err != nil {
		return nil, false, err
	}
	if dav {
		u, err := url.JoinPath(endpoint, davPath)
		if err != nil {
			return nil, false, err
		}
		return &backend.Target{URL: u, Header: http.Header{appctx.TokenHeader: {davToken}}}, true, nil
	}
	h := http.Header{}
	h.Set(datagateway.TokenTransportHeader, token)
	if t, ok := appctx.ContextGetToken(ctx); ok {
		h.Set(appctx.TokenHeader, t)
	}
	return &backend.Target{URL: endpoint, Header: h}, false, nil
}

// webdavRef returns the WebDAV token and file path that some storage
// providers put in the opaque map of the simple protocol.
func webdavRef(o *types.Opaque) (token, filePath string, ok bool, err error) {
	if o == nil {
		return "", "", false, nil
	}
	for key, dst := range map[string]*string{"webdav-token": &token, "webdav-file-path": &filePath} {
		e, found := o.Map[key]
		if !found {
			return "", "", false, nil
		}
		if e.Decoder != "plain" {
			return "", "", false, errors.New("opaque entry decoder not recognized: " + e.Decoder)
		}
		*dst = string(e.Value)
	}
	return token, filePath, true, nil
}

func (l locator) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	opaque := &types.Opaque{Map: map[string]*types.OpaqueEntry{}}
	if opts.Insecure {
		opaque.Map["insecure"] = &types.OpaqueEntry{}
	}
	if opts.SkipVerify {
		opaque.Map["skip-verify"] = &types.OpaqueEntry{}
	}
	res, err := check(c.OpenInApp(ctx, &gateway.OpenInAppRequest{Ref: ref, ViewMode: opts.ViewMode, App: opts.App, Opaque: opaque}))
	if err != nil {
		return nil, err
	}
	return res.AppUrl, nil
}
