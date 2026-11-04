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

const simpleProtocol = "simple"

type locator struct{ b *Backend }

func (l locator) DownloadTarget(ctx context.Context, ref *provider.Reference) (*backend.Target, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{Ref: ref}))
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

// target returns the data gateway, or the WebDAV path that some storage
// providers put in the opaque map instead.
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
	opaque := &types.Opaque{Map: map[string]*types.OpaqueEntry{}}
	if opts.Insecure {
		opaque.Map["insecure"] = &types.OpaqueEntry{}
	}
	if opts.SkipVerify {
		opaque.Map["skip-verify"] = &types.OpaqueEntry{}
	}
	res, err := call(ctx, l.b, gateway.GatewayAPIClient.OpenInApp, &gateway.OpenInAppRequest{Ref: ref, ViewMode: opts.ViewMode, App: opts.App, Opaque: opaque})
	if err != nil {
		return nil, err
	}
	return res.AppUrl, nil
}
