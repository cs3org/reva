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
	"io"
	"net/http"
	"path"
	"path/filepath"

	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils/resourceid"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

// VersionsHandler handles version requests
type VersionsHandler struct {
}

func (h *VersionsHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
// versions can be listed with a PROPFIND to /remote.php/dav/meta/<fileid>/v
// a version is identified by a timestamp, eg. /remote.php/dav/meta/<fileid>/v/1561410426
func (h *VersionsHandler) Handler(s *svc, rid *provider.ResourceId) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if rid == nil {
			http.Error(w, "404 Not Found", http.StatusNotFound)
			return
		}

		// baseURI is encoded as part of the response payload in href field
		baseURI := path.Join(ctx.Value(ctxKeyBaseURI).(string), resourceid.OwnCloudResourceIDWrap(rid))
		ctx = context.WithValue(ctx, ctxKeyBaseURI, baseURI)
		r = r.WithContext(ctx)

		var key string
		key, r.URL.Path = router.ShiftPath(r.URL.Path)
		if r.Method == http.MethodOptions {
			s.handleOptions(w, r)
			return
		}
		if key == "" && r.Method == MethodPropfind {
			h.doListVersions(w, r, s, rid)
			return
		}
		if key != "" && r.Method == MethodCopy {
			// TODO(jfd) it seems we cannot directly GET version content with cs3 ...
			// TODO(jfd) cs3api has no delete file version call
			// TODO(jfd) restore version to given Destination, but cs3api has no destination
			h.doRestore(w, r, s, rid, key)
			return
		}
		if key != "" && r.Method == http.MethodGet {
			h.doDownload(w, r, s, rid, key)
			return
		}

		http.Error(w, "501 Forbidden", http.StatusNotImplemented)
	})
}

func (h *VersionsHandler) doListVersions(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "listVersions")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Interface("resourceid", rid).Logger()

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &provider.Reference{ResourceId: rid}
	res, err := client.Stat(ctx, &provider.StatRequest{Ref: ref})
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			w.WriteHeader(http.StatusNotFound)
			b, err := Marshal(exception{
				code:    SabredavNotFound,
				message: "Resource not found",
			})
			HandleWebdavError(&sublog, w, b, err)
			return
		}
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	info := res.Info

	lvRes, err := client.ListFileVersions(ctx, &provider.ListFileVersionsRequest{Ref: ref})
	if err != nil {
		sublog.Error().Err(err).Msg("error sending list container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if lvRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, lvRes.Status)
		return
	}

	versions := lvRes.GetVersions()
	infos := make([]*provider.ResourceInfo, 0, len(versions)+1)
	// add version dir . entry, derived from file info
	infos = append(infos, &provider.ResourceInfo{
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
	})

	for i := range versions {
		vi := &provider.ResourceInfo{
			// TODO(jfd) we cannot access version content, this will be a problem when trying to fetch version thumbnails
			// Opaque
			Type: provider.ResourceType_RESOURCE_TYPE_FILE,
			Id: &provider.ResourceId{
				StorageId: "versions",
				OpaqueId:  info.Id.OpaqueId + "@" + versions[i].GetKey(),
			},
			// Checksum
			Etag: versions[i].Etag,
			// MimeType
			Mtime: &types.Timestamp{
				Seconds: versions[i].Mtime,
				// TODO cs3apis FileVersion should use types.Timestamp instead of uint64
			},
			Path: path.Join("v", versions[i].Key),
			// PermissionSet
			Size:  versions[i].Size,
			Owner: info.Owner,
		}
		infos = append(infos, vi)
	}

	propRes, err := s.multistatusResponse(ctx, &pf, infos, "", nil, nil)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(HeaderContentType, "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, err = w.Write([]byte(propRes))
	if err != nil {
		sublog.Error().Err(err).Msg("error writing body")
		return
	}

}

func (h *VersionsHandler) doRestore(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId, key string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "restore")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Interface("resourceid", rid).Str("key", key).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &provider.RestoreFileVersionRequest{
		Ref: &provider.Reference{ResourceId: rid},
		Key: key,
	}

	res, err := client.RestoreFileVersion(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc restore version request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VersionsHandler) doDownload(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId, key string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "restore")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Interface("resourceid", rid).Str("key", key).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resStat, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: rid,
		},
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if resStat.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, resStat.Status)
		return
	}

	dir, fname := filepath.Split(resStat.GetInfo().Path)

	versionFile := filepath.Join(dir, ".sys.v#."+fname, key)

	down := downloader.NewDownloader(client, rhttp.Context(ctx))
	reader, err := down.Download(ctx, versionFile)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fname))
	w.Header().Set("Content-Transfer-Encoding", "binary")
	io.Copy(w, reader)
}
