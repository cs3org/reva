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

package ocdav

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/internal/http/utils"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// TrashbinHandler handles trashbin requests
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
			s.doOptions(w, r, "trashbin")
			return
		}

		var username string
		username, r.URL.Path = rhttp.ShiftPath(r.URL.Path)

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

		// key is the fileid ... TODO call it fileid? there is a difference to the trashbin key we got from the cs3 api
		var key string
		key, r.URL.Path = rhttp.ShiftPath(r.URL.Path)
		if r.Method == http.MethodOptions {
			s.doOptions(w, r, "trashbin")
			return
		}
		if key == "" && r.Method == "PROPFIND" {
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

			// baseURI is encoded as part of the response payload in href field
			baseURI := path.Join(ctx.Value(ctxKeyBaseURI).(string), "files", username)
			ctx = context.WithValue(ctx, ctxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			log.Info().Str("url_path", urlPath).Str("base_uri", baseURI).Msg("move urls")
			// TODO make request.php optional in destination header
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

func (h *TrashbinHandler) listTrashbin(w http.ResponseWriter, r *http.Request, s *svc, u *userproviderv0alphapb.User) {
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

	lrReq := &gatewayv0alphapb.ListRecycleRequest{
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
	propRes, err := h.formatTrashPropfind(ctx, s, u, &pf, lrRes.GetRecycleItems())
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

func (h *TrashbinHandler) formatTrashPropfind(ctx context.Context, s *svc, u *userproviderv0alphapb.User, pf *propfindXML, items []*storageproviderv0alphapb.RecycleItem) (string, error) {
	responses := make([]*responseXML, 0, len(items)+1)
	// add trashbin dir . entry
	responses = append(responses, &responseXML{
		Href: (&url.URL{Path: ctx.Value(ctxKeyBaseURI).(string) + "/"}).EscapedPath(), // url encode response.Href TODO (jfd) really? /should be ok ... we may actually only need to escape the username
		Propstat: []propstatXML{
			propstatXML{
				Status: "HTTP/1.1 200 OK",
				Prop: []*propertyXML{
					s.newProp("d:resourcetype", "<d:collection/>"),
				},
			},
			propstatXML{
				Status: "HTTP/1.1 404 Not Found",
				Prop: []*propertyXML{
					s.newProp("oc:trashbin-original-filename", ""),
					s.newProp("oc:trashbin-original-location", ""),
					s.newProp("oc:trashbin-delete-datetime", ""),
					s.newProp("d:getcontentlength", ""),
				},
			},
		},
	})
	/*
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
	*/
	for i := range items {
		res, err := h.itemToPropResponse(ctx, s, pf, items[i])
		if err != nil {
			return "", err
		}
		responses = append(responses, res)
	}
	responsesXML, err := xml.Marshal(&responses)
	if err != nil {
		return "", err
	}

	msg := `<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" `
	msg += `xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">`
	msg += string(responsesXML) + `</d:multistatus>`
	return msg, nil
}

func (h *TrashbinHandler) itemToPropResponse(ctx context.Context, s *svc, pf *propfindXML, item *storageproviderv0alphapb.RecycleItem) (*responseXML, error) {

	baseURI := ctx.Value(ctxKeyBaseURI).(string)
	ref := path.Join(baseURI, item.Key)
	if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := responseXML{
		Href:     (&url.URL{Path: ref}).EscapedPath(), // url encode response.Href
		Propstat: []propstatXML{},
	}

	// TODO(jfd): if the path we list here is taken from the ListRecycle request we rely on the gateway to prefix it with the mount point

	t := utils.TSToTime(item.DeletionTime).UTC()
	dTime := t.Format(time.RFC1123)

	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties
		response.Propstat = append(response.Propstat, propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		})
		// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-original-filename", path.Base(item.Path)))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-original-location", item.Path))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-delete-datetime", dTime))
		if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:resourcetype", "<d:collection/>"))
			// TODO(jfd): decide if we can and want to list oc:size for folders
		} else {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				s.newProp("d:resourcetype", ""),
				s.newProp("d:getcontentlength", fmt.Sprintf("%d", item.Size)),
			)
		}

	} else {
		// otherwise return only the requested properties
		propstatOK := propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		}
		propstatNotFound := propstatXML{
			Status: "HTTP/1.1 404 Not Found",
			Prop:   []*propertyXML{},
		}
		size := fmt.Sprintf("%d", item.Size)
		for i := range pf.Prop {
			switch pf.Prop[i].Space {
			case "http://owncloud.org/ns":
				switch pf.Prop[i].Local {
				case "oc:size":
					if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:size", ""))
					}
				case "trashbin-original-filename":
					// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-original-filename", path.Base(item.Path)))
				case "trashbin-original-location":
					// TODO (jfd) double check and clarify the cs3 spec what the Key is about and if Path is only the folder that contains the file or if it includes the filename
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-original-location", item.Path))
				case "trashbin-delete-datetime":
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-delete-datetime", dTime))
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
				}
			case "DAV:":
				switch pf.Prop[i].Local {
				case "getcontentlength":
					if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getcontentlength", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
					}
				case "resourcetype":
					if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype":
					if item.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", "httpd/unix-directory"))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getcontenttype", ""))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:"+pf.Prop[i].Local, ""))
				}
			default:
				// TODO (jfd) lookup shortname for unknown namespaces?
				propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp(pf.Prop[i].Space+":"+pf.Prop[i].Local, ""))
			}
		}
		response.Propstat = append(response.Propstat, propstatOK, propstatNotFound)
	}

	return &response, nil
}

func (h *TrashbinHandler) restore(w http.ResponseWriter, r *http.Request, s *svc, u *userproviderv0alphapb.User, dst string, key string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.RestoreRecycleItemRequest{
		/* TODO(jfd) Ref is required ... but which resource should it reference?
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
		},
		*/
		Key:         key,
		RestorePath: dst,
	}

	res, err := client.RestoreRecycleItem(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc restore recycle item request")
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
