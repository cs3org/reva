// Copyright 2018-2020 CERN
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

package ocdav

import (
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/cs3org/reva/internal/http/services/datagateway"
	"go.opencensus.io/trace"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/utils"
)

func (s *svc) handleGet(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	ns = applyLayout(ctx, ns)

	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
	}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		handleErrorStatus(&sublog, w, sRes.Status)
		return
	}

	info := sRes.Info
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		sublog.Warn().Msg("resource is a folder and cannot be downloaded")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	dReq := &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
	}

	dRes, err := client.InitiateFileDownload(ctx, dReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error initiating file download")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dRes.Status.Code != rpc.Code_CODE_OK {
		handleErrorStatus(&sublog, w, dRes.Status)
		return
	}

	var ep, token string
	for _, p := range dRes.Protocols {
		if p.Protocol == "simple" {
			ep, token = p.DownloadEndpoint, p.Token
		}
	}

	httpReq, err := rhttp.NewRequest(ctx, "GET", ep, nil)
	if err != nil {
		sublog.Error().Err(err).Msg("error creating http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)
	httpClient := s.client

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error performing http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+
		path.Base(info.Path)+"; filename=\""+path.Base(info.Path)+"\"")
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id))
	w.Header().Set("OC-ETag", info.Etag)
	t := utils.TSToTime(info.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set("Last-Modified", lastModifiedString)
	w.Header().Set("Content-Length", strconv.FormatUint(info.Size, 10))
	/*
		if md.Checksum != "" {
			w.Header().Set("OC-Checksum", md.Checksum)
		}
	*/
	if _, err := io.Copy(w, httpRes.Body); err != nil {
		sublog.Error().Err(err).Msg("error finishing copying data to response")
	}
}
