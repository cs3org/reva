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
	"net/http"
	"path"
	"strconv"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/utils"
	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) handleHead(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	ns = applyLayout(ctx, ns)

	fn := path.Join(ns, r.URL.Path)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	req := &provider.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", fn).Interface("status", res.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", fn).Interface("status", res.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", fn).Interface("status", res.Status).Msg("grpc stat request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	info := res.Info
	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id))
	w.Header().Set("OC-ETag", info.Etag)
	t := utils.TSToTime(info.Mtime)
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set("Last-Modified", lastModifiedString)
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Length", strconv.FormatUint(info.Size, 10))
}
