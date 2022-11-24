// Copyright 2018-2021 CERN
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
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/pkg/appctx"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
)

func (s *svc) handlePathHead(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "head")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	ref := &provider.Reference{Path: fn}
	s.handleHead(ctx, w, r, ref, sublog)
}

func (s *svc) handleHead(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, log zerolog.Logger) {
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &provider.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&log, w, res.Status)
		return
	}

	info := res.Info
	w.Header().Set(HeaderContentType, info.MimeType)
	w.Header().Set(HeaderETag, info.Etag)
	w.Header().Set(HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(info.Id))
	w.Header().Set(HeaderOCETag, info.Etag)
	if info.Checksum != nil {
		w.Header().Set(HeaderOCChecksum, fmt.Sprintf("%s:%s", strings.ToUpper(string(storageprovider.GRPC2PKGXS(info.Checksum.Type))), info.Checksum.Sum))
		w.Header().Set(HeaderChecksum, fmt.Sprintf("%s=%s", strings.ToLower(string(storageprovider.GRPC2PKGXS(info.Checksum.Type))), info.Checksum.Sum))
	}
	t := utils.TSToTime(info.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set(HeaderLastModified, lastModifiedString)
	w.Header().Set(HeaderContentLength, strconv.FormatUint(info.Size, 10))
	if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		w.Header().Set(HeaderAcceptRanges, "bytes")
	}
	w.WriteHeader(http.StatusOK)
}

func (s *svc) handleSpacesHead(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "spaces_head")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()

	spaceRef, status, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handleHead(ctx, w, r, spaceRef, sublog)
}
