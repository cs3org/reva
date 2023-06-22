// Copyright 2018-2023 CERN
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

package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
)

// Downloader is the interface implemented by the objects that are able to
// download a path into a destination Writer.
type Downloader interface {
	Download(ctx context.Context, path, versionKey string) (io.ReadCloser, error)
}

type revaDownloader struct {
	gtw        gateway.GatewayAPIClient
	httpClient *http.Client
}

// NewDownloader creates a Downloader from the reva gateway.
func NewDownloader(gtw gateway.GatewayAPIClient, options ...rhttp.Option) Downloader {
	return &revaDownloader{
		gtw:        gtw,
		httpClient: rhttp.GetHTTPClient(options...),
	}
}

func getDownloadProtocol(protocols []*gateway.FileDownloadProtocol, prot string) (*gateway.FileDownloadProtocol, error) {
	for _, p := range protocols {
		if p.Protocol == prot {
			return p, nil
		}
	}
	return nil, errtypes.InternalError(fmt.Sprintf("protocol %s not supported for downloading", prot))
}

// Download downloads a resource given the path to the dst Writer.
func (r *revaDownloader) Download(ctx context.Context, path, versionKey string) (io.ReadCloser, error) {
	req := &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	}
	if versionKey != "" {
		req.Opaque = &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"version_key": {
					Decoder: "plain",
					Value:   []byte(versionKey),
				},
			},
		}
	}
	downResp, err := r.gtw.InitiateFileDownload(ctx, req)

	switch {
	case err != nil:
		return nil, err
	case downResp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(downResp.Status.Message)
	}

	p, err := getDownloadProtocol(downResp.Protocols, "simple")
	if err != nil {
		return nil, err
	}

	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, p.DownloadEndpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)

	httpRes, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpRes.StatusCode != http.StatusOK {
		defer httpRes.Body.Close()
		switch httpRes.StatusCode {
		case http.StatusNotFound:
			return nil, errtypes.NotFound(path)
		default:
			return nil, errtypes.InternalError(httpRes.Status)
		}
	}

	return httpRes.Body, nil
}
