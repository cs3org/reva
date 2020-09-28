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
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/utils"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// TrashbinHandler handles trashbin requests
type TrashbinHandler struct {
	gatewaySvc string
}

func (h *TrashbinHandler) init(c *Config) error {
	h.gatewaySvc = c.GatewaySvc
	return nil
}

// Handler handles requests
func (h *TrashbinHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.handleOptions(w, r, "trashbin")
			return
		}

		var userid string
		userid, r.URL.Path = router.ShiftPath(r.URL.Path)

		if userid == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		u, ok := ctxuser.ContextGetUser(ctx)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if u.Id.OpaqueId != userid {
			log.Debug().Str("userid", userid).Interface("user", u).Msg("trying to read another users trash")
			// listing other users trash is forbidden, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// key will be a base63 encoded cs3 path, it uniquely identifies a trash item & storage
		var key string
		key, r.URL.Path = router.ShiftPath(r.URL.Path)

		// TODO another options handler should not be necessary
		//if r.Method == http.MethodOptions {
		//	s.doOptions(w, r, "trashbin")
		//	return
		//}

		if key == "" && r.Method == "PROPFIND" {
			h.listTrashbin(w, r, s, u)
			return
		}
		if key != "" && r.Method == "MOVE" {
			// find path in url relative to trash base
			trashBase := ctx.Value(ctxKeyBaseURI).(string)
			baseURI := path.Join(path.Dir(trashBase), "files", userid)
			ctx = context.WithValue(ctx, ctxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			// TODO make request.php optional in destination header
			dstHeader := r.Header.Get("Destination")
			dst, err := extractDestination(dstHeader, baseURI)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			dst = path.Clean(dst)

			log.Debug().Str("key", key).Str("dst", dst).Msg("restore")

			h.restore(w, r, s, u, dst, key)
			return
		}
		if key != "" && r.Method == "DELETE" {
			h.delete(w, r, s, u, key)
			return
		}

		http.Error(w, "501 Forbidden", http.StatusNotImplemented)
	})
}

func (h *TrashbinHandler) listTrashbin(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	gc, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
	if err != nil {
		// TODO(jfd) how do we make the user aware that some storages are not available?
		// opaque response property? Or a list of errors?
		// add a recycle entry with the path to the storage that produced the error?
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getHomeRes, err := gc.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error calling GetHome")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if getHomeRes.Status.Code != rpc.Code_CODE_OK {
		switch getHomeRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("grpc get home request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// ask gateway for recycle items
	// TODO(labkode): add Reference to ListRecycleRequest
	getRecycleRes, err := gc.ListRecycle(ctx, &gateway.ListRecycleRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: getHomeRes.Path,
			},
		},
	})

	if err != nil {
		log.Error().Err(err).Msg("error calling ListRecycle")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if getRecycleRes.Status.Code != rpc.Code_CODE_OK {
		switch getRecycleRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getRecycleRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getRecycleRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", getHomeRes.Path).Interface("status", getRecycleRes.Status).Msg("grpc list recycle request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	propRes, err := h.formatTrashPropfind(ctx, s, u, &pf, getRecycleRes.RecycleItems)
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

func (h *TrashbinHandler) formatTrashPropfind(ctx context.Context, s *svc, u *userpb.User, pf *propfindXML, items []*provider.RecycleItem) (string, error) {
	responses := make([]*responseXML, 0, len(items)+1)
	// add trashbin dir . entry
	responses = append(responses, &responseXML{
		Href: (&url.URL{Path: ctx.Value(ctxKeyBaseURI).(string) + "/"}).EscapedPath(), // url encode response.Href TODO (jfd) really? /should be ok ... we may actually only need to escape the username
		Propstat: []propstatXML{
			{
				Status: "HTTP/1.1 200 OK",
				Prop: []*propertyXML{
					s.newProp("d:resourcetype", "<d:collection/>"),
				},
			},
			{
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

// itemToPropResponse needs to create a listing that contains a key and destination
// the key is the name of an entry in the trash listing
// for now we need to limit trash to the users home, so we can expect all trash keys to have the home storage as the opaque id
func (h *TrashbinHandler) itemToPropResponse(ctx context.Context, s *svc, pf *propfindXML, item *provider.RecycleItem) (*responseXML, error) {

	baseURI := ctx.Value(ctxKeyBaseURI).(string)
	ref := path.Join(baseURI, item.Key)
	if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := responseXML{
		Href:     (&url.URL{Path: ref}).EscapedPath(), // url encode response.Href
		Propstat: []propstatXML{},
	}

	// TODO(jfd): if the path we list here is taken from the ListRecycle request we rely on the gateway to prefix it with the mount point

	t := utils.TSToTime(item.DeletionTime).UTC()
	dTime := t.Format(time.RFC1123Z)

	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties
		response.Propstat = append(response.Propstat, propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		})
		// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-original-filename", strings.TrimPrefix(item.Path, "/")))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-original-location", strings.TrimPrefix(item.Path, "/")))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:trashbin-delete-datetime", dTime))
		if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
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
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:size", ""))
					}
				case "trashbin-original-filename":
					// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-original-filename", strings.TrimPrefix(item.Path, "/")))
				case "trashbin-original-location":
					// TODO (jfd) double check and clarify the cs3 spec what the Key is about and if Path is only the folder that contains the file or if it includes the filename
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-original-location", strings.TrimPrefix(item.Path, "/")))
				case "trashbin-delete-datetime":
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:trashbin-delete-datetime", dTime))
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
				}
			case "DAV:":
				switch pf.Prop[i].Local {
				case "getcontentlength":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getcontentlength", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
					}
				case "resourcetype":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
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

// restore has a destination and a key
func (h *TrashbinHandler) restore(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, dst string, key string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getHomeRes, err := client.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error calling GetHome")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if getHomeRes.Status.Code != rpc.Code_CODE_OK {
		switch getHomeRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("grpc get home request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	req := &provider.RestoreRecycleItemRequest{
		// use the target path to find the storage provider
		// this means we can only undelete on the same storage, not to a different folder
		// use the key which is prefixed with the StoragePath to lookup the correct storage ...
		// TODO currently limited to the home storage
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: getHomeRes.Path,
			},
		},
		Key:         key,
		RestorePath: dst,
	}

	res, err := client.RestoreRecycleItem(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc restore recycle item request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusNoContent)
	case rpc.Code_CODE_NOT_FOUND:
		log.Debug().Str("key", key).Str("dst", dst).Interface("status", res.Status).Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
	case rpc.Code_CODE_PERMISSION_DENIED:
		log.Debug().Str("key", key).Str("dst", dst).Interface("status", res.Status).Msg("permission denied")
		w.WriteHeader(http.StatusForbidden)
	default:
		log.Error().Str("key", key).Str("dst", dst).Interface("status", res.Status).Msg("grpc restore recycle item request failed")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// delete has only a key
func (h *TrashbinHandler) delete(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, key string) {

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getHomeRes, err := client.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error calling GetHomeProvider")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if getHomeRes.Status.Code != rpc.Code_CODE_OK {
		switch getHomeRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", getHomeRes.Path).Interface("status", getHomeRes.Status).Msg("grpc get home request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	sRes, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: getHomeRes.Path,
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error calling Stat")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if sRes.Status.Code != rpc.Code_CODE_OK {
		switch sRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", sRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", getHomeRes.Path).Interface("status", sRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", getHomeRes.Path).Interface("status", sRes.Status).Msg("grpc stat request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// set key as opaque id, the storageprovider will use it as the key for the
	// storage drives  PurgeRecycleItem key call

	req := &gateway.PurgeRecycleRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: &provider.ResourceId{
					OpaqueId:  key,
					StorageId: sRes.Info.Id.StorageId,
				},
			},
		},
	}

	res, err := client.PurgeRecycle(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc restore recycle item request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusNoContent)
	case rpc.Code_CODE_NOT_FOUND:
		log.Debug().Str("storageid", sRes.Info.Id.StorageId).Str("key", key).Interface("status", res.Status).Msg("resource not found")
		w.WriteHeader(http.StatusConflict)
	case rpc.Code_CODE_PERMISSION_DENIED:
		log.Debug().Str("storageid", sRes.Info.Id.StorageId).Str("key", key).Interface("status", res.Status).Msg("permission denied")
		w.WriteHeader(http.StatusForbidden)
	default:
		log.Error().Str("storageid", sRes.Info.Id.StorageId).Str("key", key).Interface("status", res.Status).Msg("grpc purge recycle request failed")
		w.WriteHeader(http.StatusInternalServerError)
	}
}
