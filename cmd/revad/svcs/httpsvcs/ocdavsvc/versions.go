// Copyright 2018-2019 CERN
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

package ocdavsvc

import (
	"context"
	"net/http"
	"path"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
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
func (h *VersionsHandler) Handler(s *svc, rid *storageproviderv0alphapb.ResourceId) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// baseURI is encoded as part of the response payload in href field
		baseURI := path.Join("/", s.Prefix(), "remote.php/dav/meta", wrapResourceID(rid))
		ctx := context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)
		r = r.WithContext(ctx)

		var key string
		key, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if r.Method == http.MethodOptions {
			s.doOptions(w, r)
			return
		}
		if key == "" && r.Method == "PROPFIND" {
			h.doListVersions(w, r, s, rid)
			return
		}
		if key != "" && r.Method == "COPY" {
			// TODO(jfd) it seems we cannot directly GET version content with cs3 ...
			// TODO(jfd) cs3api has no delete file version call
			// TODO(jfd) restore version to given Destination, but cs3api has no destination
			h.doRestore(w, r, s, rid, key)
			return
		}

		http.Error(w, "501 Forbidden", http.StatusNotImplemented)
	})
}

func (h *VersionsHandler) doListVersions(w http.ResponseWriter, r *http.Request, s *svc, rid *storageproviderv0alphapb.ResourceId) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Id{Id: rid},
	}
	req := &storageproviderv0alphapb.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	info := res.Info

	lvReq := &storageproviderv0alphapb.ListFileVersionsRequest{
		Ref: ref,
	}
	lvRes, err := client.ListFileVersions(ctx, lvReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending list container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if lvRes.Status.Code != rpcpb.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	versions := lvRes.GetVersions()
	infos := make([]*storageproviderv0alphapb.ResourceInfo, 0, len(versions)+1)
	// add version dir . entry, derived from file info
	infos = append(infos, &storageproviderv0alphapb.ResourceInfo{
		Type: storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER,
		Id: &storageproviderv0alphapb.ResourceId{
			StorageId: "virtual", // this is a virtual storage
			OpaqueId:  path.Join("meta", wrapResourceID(rid), "v"),
		},
		Etag:     info.Etag,
		MimeType: "httpd/unix-directory",
		Mtime:    info.Mtime,
		Path:     "v",
		//PermissionSet
		Size:  0,
		Owner: info.Owner,
	})

	for i := range versions {
		vi := &storageproviderv0alphapb.ResourceInfo{
			// TODO(jfd) we cannot access version content, this will be a problem when trying to fetch version thumbnails
			//Opaque
			Type: storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE,
			Id: &storageproviderv0alphapb.ResourceId{
				StorageId: "versions", // this is a virtual storage
				OpaqueId:  info.Id.OpaqueId + "@" + versions[i].GetKey(),
			},
			//Checksum
			//Etag: v.ETag,
			//MimeType
			Mtime: &typespb.Timestamp{
				Seconds: versions[i].Mtime,
				// TODO cs3apis FileVersion should use typespb.Timestamp instead of uint64
			},
			Path: path.Join("v", versions[i].Key),
			//PermissionSet
			Size:  versions[i].Size,
			Owner: info.Owner,
		}
		infos = append(infos, vi)
	}

	propRes, err := s.formatPropfind(ctx, &pf, infos)
	if err != nil {
		log.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, err = w.Write([]byte(propRes))
	if err != nil {
		log.Error().Err(err).Msg("error writing body")
		return
	}

}

func (h *VersionsHandler) doRestore(w http.ResponseWriter, r *http.Request, s *svc, rid *storageproviderv0alphapb.ResourceId, key string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.RestoreFileVersionRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Id{Id: rid},
		},
		Key: key,
	}

	res, err := client.RestoreFileVersion(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc restore version request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
