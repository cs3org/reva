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
func (h *VersionsHandler) Handler(s *svc, fileid string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var timestamp string
		timestamp, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if timestamp == "" && r.Method == "PROPFIND" {
			// TODO(jfd) list versions
			h.doPropfind(w, r, s, fileid)
			return
		}
		if timestamp != "" {
			// TODO(jfd) version operations
			// TODO(jfd) we need to use the fileid and directly interact with the storage

			switch r.Method {
			case "PROPFIND":
				h.doPropfind(w, r, s, fileid)
			//case "HEAD": // TODO(jfd) since we cant GET ... there is no HEAD
			//	s.doHead(w, r)
			case "GET": // TODO(jfd) it seems we cannot directly GET version content with cs3 ...
				s.doGet(w, r)
			case "COPY": // TODO(jfd) restore version to Destination, but cs3api has no destination
				s.doCopy(w, r)
			case "DELETE": // TODO(jfd) cs3api has no delete file version call
				s.doDelete(w, r)
			default:
				http.Error(w, "403 Forbidden", http.StatusForbidden)
			}
			return
		}

		http.Error(w, "403 Forbidden", http.StatusForbidden)
	})
}

func (h *VersionsHandler) doPropfind(w http.ResponseWriter, r *http.Request, s *svc, fileid string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	_, status, err := readPropfind(r.Body)
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
		Spec: &storageproviderv0alphapb.Reference_Id{
			Id: &storageproviderv0alphapb.ResourceId{
				// StorageId: TODO ??? where do we get that from?
				// somewhere in the ocdavapi we need to resolve the ids
				OpaqueId: fileid,
			},
		},
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
	infos := []*storageproviderv0alphapb.ResourceInfo{info}

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
	for _, v := range lvRes.GetVersions() {
		vi := &storageproviderv0alphapb.ResourceInfo{
			// TODO(jfd) we cannot access version content, this will be a problem when trying to fetch version thumbnails
			//Opaque
			Type: storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE,
			Id: &storageproviderv0alphapb.ResourceId{
				StorageId: "versions", // this is a virtual storage
				OpaqueId:  info.Id.OpaqueId + "@" + v.GetKey(),
			},
			//Checksum
			//Etag: v.ETag,
			//MimeType
			Mtime: &typespb.Timestamp{
				Seconds: v.Mtime,
				// TODO cs3apis FileVersion should use typespb.Timestamp instead of uint64
			},
			Path: path.Join("/", s.Prefix(), "remote.php/dav/meta", fileid, "v", v.Key),
			//PermissionSet
			Size:  v.Size,
			Owner: info.Owner,
		}
		infos = append(infos, vi)
	}

	propRes, err := s.formatPropfind(ctx, infos)
	if err != nil {
		log.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(propRes))
}
