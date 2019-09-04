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
	"net/url"
	"path"
	"strings"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// TrashbinHandler handles version requests
type TrashbinHandler struct {
}

func (h *TrashbinHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
func (h *TrashbinHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.doOptions(w, r)
			return
		}

		var username string
		username, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

		if username == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		u, ok := ctxuser.ContextGetUser(ctx)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if u.Username != username {
			// listing other users trash is forbidden, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// key is the fileid ... TODO call it fileid? there is a difference to the treshbin key we got from the cs3 api
		var key string
		key, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if r.Method == http.MethodOptions {
			s.doOptions(w, r)
			return
		}
		if key == "" && r.Method == "PROPFIND" {

			// webdav should be death: baseURI is encoded as part of the
			// response payload in href field
			baseURI := path.Join("/", s.Prefix(), "remote.php/dav/trash-bin", username)
			ctx = context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			h.listTrashbin(w, r, s, u)
			return
		}
		if key != "" && r.Method == "MOVE" {
			dstHeader := r.Header.Get("Destination")

			log.Info().Str("key", key).Str("dst", dstHeader).Msg("restore")

			if dstHeader == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// strip baseURL from destination
			dstURL, err := url.ParseRequestURI(dstHeader)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			urlPath := dstURL.Path

			// webdav should be death: baseURI is encoded as part of the
			// response payload in href field
			baseURI := path.Join("/", s.Prefix(), "remote.php/dav/files", username)
			ctx = context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			log.Info().Str("url_path", urlPath).Str("base_uri", baseURI).Msg("move urls")
			i := strings.Index(urlPath, baseURI)
			if i == -1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			dst := path.Clean(urlPath[len(baseURI):])

			h.restore(w, r, s, u, dst, key)
			return
		}

		http.Error(w, "501 Forbidden", http.StatusNotImplemented)
	})
}

func (h *TrashbinHandler) listTrashbin(w http.ResponseWriter, r *http.Request, s *svc, u *authv0alphapb.User) {
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

	lrReq := &storageproviderv0alphapb.ListRecycleRequest{
		// TODO implement from to?
		//FromTs
		//ToTs
	}
	lrRes, err := client.ListRecycle(ctx, lrReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending list container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if lrRes.Status.Code != rpcpb.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	items := lrRes.GetRecycleItems()
	infos := make([]*storageproviderv0alphapb.ResourceInfo, 0, len(items)+1)
	// add trashbin dir . entry, derived from file info
	infos = append(infos, &storageproviderv0alphapb.ResourceInfo{
		Type: storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER,
		Id: &storageproviderv0alphapb.ResourceId{
			StorageId: "trashbin", // this is a virtual storage
			OpaqueId:  path.Join("trash-bin", u.Username),
		},
		//Etag:     info.Etag,
		MimeType: "httpd/unix-directory",
		//Mtime:    info.Mtime,
		Path: u.Username,
		//PermissionSet
		Size:  0,
		Owner: u.Id,
	})

	for i := range items {
		vi := &storageproviderv0alphapb.ResourceInfo{
			// TODO(jfd) we cannot access version content, this will be a problem when trying to fetch version thumbnails
			//Opaque
			Type: storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE,
			Id: &storageproviderv0alphapb.ResourceId{
				StorageId: "trashbin", // this is a virtual storage
				OpaqueId:  path.Join("trash-bin", u.Username, items[i].GetKey()),
			},
			//Checksum
			//Etag: v.ETag,
			//MimeType
			Mtime: items[i].DeletionTime,
			Path:  items[i].Key,
			//PermissionSet
			Size:  items[i].Size,
			Owner: u.Id,
		}
		infos = append(infos, vi)
	}

	// TODO(jfd) render trashbin response
	// <oc:trashbin-original-filename>ownCloud Manual.pdf</oc:trashbin-original-filename>
	// this seems to be relative to the users home ... which is bad: now the client has to build the proper Destination url
	// <oc:trashbin-original-location>ownCloud Manual.pdf</oc:trashbin-original-location>
	// <oc:trashbin-delete-datetime>Thu, 29 Aug 2019 14:06:24 GMT</oc:trashbin-delete-datetime>
	// d:getcontentlength
	// d:resourcetype
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

func (h *TrashbinHandler) restore(w http.ResponseWriter, r *http.Request, s *svc, u *authv0alphapb.User, dst string, key string) {
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
			Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
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
