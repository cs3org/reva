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

package ocdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
)

func (s *svc) handlePathGet(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "get")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Str("svc", "ocdav").Str("handler", "get").Logger()

	ref := &provider.Reference{Path: fn}
	s.handleGet(ctx, w, r, ref, "simple", sublog)
}

func (s *svc) handleGet(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, dlProtocol string, log zerolog.Logger) {
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{Ref: ref}
	sRes, err := client.Stat(ctx, sReq)
	switch {
	case err != nil:
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	case sRes.Status.Code != rpc.Code_CODE_OK:
		HandleErrorStatus(&log, w, sRes.Status)
		return
	case sRes.Info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		log.Warn().Msg("resource is a folder and cannot be downloaded")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	dReq := &provider.InitiateFileDownloadRequest{Ref: ref}
	dRes, err := client.InitiateFileDownload(ctx, dReq)
	if err != nil {
		log.Error().Err(err).Msg("error initiating file download")
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if dRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&log, w, dRes.Status)
		return
	}

	var ep, token string
	for _, p := range dRes.Protocols {
		if p.Protocol == dlProtocol {
			ep, token = p.DownloadEndpoint, p.Token
		}
	}

	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, ep, nil)
	if err != nil {
		log.Error().Err(err).Msg("error creating http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	if r.Header.Get(HeaderRange) != "" {
		httpReq.Header.Set(HeaderRange, r.Header.Get(HeaderRange))
	}

	httpClient := s.client

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("error performing http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK && httpRes.StatusCode != http.StatusPartialContent {
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	info := sRes.Info
	filename := path.Base(r.URL.Path)

	w.Header().Set(HeaderContentType, info.MimeType)
	w.Header().Set(HeaderContentDisposistion, "attachment; filename=\""+filename+"\"")
	w.Header().Set(HeaderETag, info.Etag)
	w.Header().Set(HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(info.Id))
	w.Header().Set(HeaderOCETag, info.Etag)
	t := utils.TSToTime(info.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set(HeaderLastModified, lastModifiedString)

	if httpRes.StatusCode == http.StatusPartialContent {
		w.Header().Set(HeaderContentRange, httpRes.Header.Get(HeaderContentRange))
		w.Header().Set(HeaderContentLength, httpRes.Header.Get(HeaderContentLength))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set(HeaderContentLength, strconv.FormatUint(info.Size, 10))
	}
	if info.Checksum != nil {
		w.Header().Set(HeaderOCChecksum, fmt.Sprintf("%s:%s", storageprovider.GRPC2PKGXS(info.Checksum.Type).String(), info.Checksum.Sum))
	}
	var c int64
	if c, err = io.Copy(w, httpRes.Body); err != nil {
		log.Error().Err(err).Msg("error finishing copying data to response")
	}
	if httpRes.Header.Get(HeaderContentLength) != "" {
		i, err := strconv.ParseInt(httpRes.Header.Get(HeaderContentLength), 10, 64)
		if err != nil {
			log.Error().Err(err).Str("content-length", httpRes.Header.Get(HeaderContentLength)).Msg("invalid content length in datagateway response")
		}
		if i != c {
			log.Error().Int64("content-length", i).Int64("transferred-bytes", c).Msg("content length vs transferred bytes mismatch")
		}
	}
	// TODO we need to send the If-Match etag in the GET to the datagateway to prevent race conditions between stating and reading the file
}

func (s *svc) handleSpacesGet(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "spaces_get")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "get").Logger()

	// retrieve a specific storage space
	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}
	s.handleGet(ctx, w, r, ref, "spaces", sublog)
}
