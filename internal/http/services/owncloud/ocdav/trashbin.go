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
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/props"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/spacelookup"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils/resourceid"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/utils"
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
			s.handleOptions(w, r)
			return
		}

		var username string
		username, r.URL.Path = router.ShiftPath(r.URL.Path)

		if username == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		u, ok := ctxpkg.ContextGetUser(ctx)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if u.Username != username {
			log.Debug().Str("username", username).Interface("user", u).Msg("trying to read another users trash")
			// listing other users trash is forbidden, no auth will change that
			w.WriteHeader(http.StatusUnauthorized)
			b, err := errors.Marshal(http.StatusUnauthorized, "", "")
			if err != nil {
				log.Error().Msgf("error marshaling xml response: %s", b)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err = w.Write(b)
			if err != nil {
				log.Error().Msgf("error writing xml response: %s", b)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		}

		// key will be a base64 encoded cs3 path, it uniquely identifies a trash item & storage
		var key string
		key, r.URL.Path = router.ShiftPath(r.URL.Path)

		// If the recycle bin corresponding to a speicific path is requested, use that.
		// If not, we user the user home to route the request
		basePath := r.URL.Query().Get("base_path")
		if basePath == "" {
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
				errors.HandleErrorStatus(log, w, getHomeRes.Status)
				return
			}
			basePath = getHomeRes.Path
		}

		if r.Method == MethodPropfind {
			h.listTrashbin(w, r, s, u, basePath, key, r.URL.Path)
			return
		}
		if key != "" && r.Method == MethodMove {
			// find path in url relative to trash base
			trashBase := ctx.Value(net.CtxKeyBaseURI).(string)
			baseURI := path.Join(path.Dir(trashBase), "files", username)
			ctx = context.WithValue(ctx, net.CtxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			// TODO make request.php optional in destination header
			dst, err := extractDestination(r)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			dst = path.Join(basePath, dst)

			log.Debug().Str("key", key).Str("dst", dst).Msg("restore")

			h.restore(w, r, s, u, basePath, dst, key, r.URL.Path)
			return
		}

		if r.Method == http.MethodDelete {
			h.delete(w, r, s, u, basePath, key, r.URL.Path)
			return
		}

		http.Error(w, "501 Not implemented", http.StatusNotImplemented)
	})
}

func (h *TrashbinHandler) listTrashbin(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, basePath, key, itemPath string) {
	ctx, span := rtrace.Provider.Tracer("trash-bin").Start(r.Context(), "list_trashbin")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Logger()

	dh := r.Header.Get(net.HeaderDepth)
	depth, err := net.ParseDepth(dh)
	if err != nil {
		sublog.Debug().Str("depth", dh).Msg(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if depth == net.DepthZero {
		propRes, err := h.formatTrashPropfind(ctx, s, u, nil, nil)
		if err != nil {
			sublog.Error().Err(err).Msg("error formatting propfind")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set(net.HeaderDav, "1, 3, extended-mkcol")
		w.Header().Set(net.HeaderContentType, "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, err = w.Write(propRes)
		if err != nil {
			sublog.Error().Err(err).Msg("error writing body")
			return
		}
		return
	}

	pf, status, err := propfind.ReadPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	gc, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
	if err != nil {
		// TODO(jfd) how do we make the user aware that some storages are not available?
		// opaque response property? Or a list of errors?
		// add a recycle entry with the path to the storage that produced the error?
		sublog.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	space, rpcstatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, basePath)
	if err != nil {
		sublog.Error().Err(err).Str("path", basePath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rpcstatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcstatus)
		return
	}
	ref := spacelookup.MakeRelativeReference(space, basePath, false)

	// ask gateway for recycle items
	getRecycleRes, err := gc.ListRecycle(ctx, &provider.ListRecycleRequest{Ref: ref, Key: path.Join(key, itemPath)})

	if err != nil {
		sublog.Error().Err(err).Msg("error calling ListRecycle")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if getRecycleRes.Status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, getRecycleRes.Status)
		return
	}

	items := getRecycleRes.RecycleItems

	if depth == net.DepthInfinity {
		var stack []string
		// check sub-containers in reverse order and add them to the stack
		// the reversed order here will produce a more logical sorting of results
		for i := len(items) - 1; i >= 0; i-- {
			// for i := range res.Infos {
			if items[i].Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
				stack = append(stack, items[i].Key)
			}
		}

		for len(stack) > 0 {
			key := stack[len(stack)-1]
			getRecycleRes, err := gc.ListRecycle(ctx, &provider.ListRecycleRequest{Ref: ref, Key: key})
			if err != nil {
				sublog.Error().Err(err).Msg("error calling ListRecycle")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if getRecycleRes.Status.Code != rpc.Code_CODE_OK {
				errors.HandleErrorStatus(&sublog, w, getRecycleRes.Status)
				return
			}
			items = append(items, getRecycleRes.RecycleItems...)

			stack = stack[:len(stack)-1]
			// check sub-containers in reverse order and add them to the stack
			// the reversed order here will produce a more logical sorting of results
			for i := len(getRecycleRes.RecycleItems) - 1; i >= 0; i-- {
				// for i := range res.Infos {
				if getRecycleRes.RecycleItems[i].Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					stack = append(stack, getRecycleRes.RecycleItems[i].Key)
				}
			}
		}
	}

	// TODO when using space based requests we should be able to get rid of this path unprefixing
	for i := range items {
		items[i].Ref.Path = strings.TrimPrefix(items[i].Ref.Path, basePath)
	}

	propRes, err := h.formatTrashPropfind(ctx, s, u, &pf, items)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(net.HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(net.HeaderContentType, "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, err = w.Write(propRes)
	if err != nil {
		sublog.Error().Err(err).Msg("error writing body")
		return
	}
}

func (h *TrashbinHandler) formatTrashPropfind(ctx context.Context, s *svc, u *userpb.User, pf *propfind.XML, items []*provider.RecycleItem) ([]byte, error) {
	responses := make([]*propfind.ResponseXML, 0, len(items)+1)
	// add trashbin dir . entry
	responses = append(responses, &propfind.ResponseXML{
		Href: net.EncodePath(ctx.Value(net.CtxKeyBaseURI).(string) + "/"), // url encode response.Href TODO
		Propstat: []propfind.PropstatXML{
			{
				Status: "HTTP/1.1 200 OK",
				Prop: []*props.PropertyXML{
					props.NewPropRaw("d:resourcetype", "<d:collection/>"),
				},
			},
			{
				Status: "HTTP/1.1 404 Not Found",
				Prop: []*props.PropertyXML{
					props.NewProp("oc:trashbin-original-filename", ""),
					props.NewProp("oc:trashbin-original-location", ""),
					props.NewProp("oc:trashbin-delete-datetime", ""),
					props.NewProp("d:getcontentlength", ""),
				},
			},
		},
	})

	for i := range items {
		res, err := h.itemToPropResponse(ctx, s, u, pf, items[i])
		if err != nil {
			return nil, err
		}
		responses = append(responses, res)
	}
	responsesXML, err := xml.Marshal(&responses)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" `)
	buf.WriteString(`xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">`)
	buf.Write(responsesXML)
	buf.WriteString(`</d:multistatus>`)
	return buf.Bytes(), nil
}

// itemToPropResponse needs to create a listing that contains a key and destination
// the key is the name of an entry in the trash listing
// for now we need to limit trash to the users home, so we can expect all trash keys to have the home storage as the opaque id
func (h *TrashbinHandler) itemToPropResponse(ctx context.Context, s *svc, u *userpb.User, pf *propfind.XML, item *provider.RecycleItem) (*propfind.ResponseXML, error) {

	baseURI := ctx.Value(net.CtxKeyBaseURI).(string)
	ref := path.Join(baseURI, u.Username, item.Key)
	if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := propfind.ResponseXML{
		Href:     net.EncodePath(ref), // url encode response.Href
		Propstat: []propfind.PropstatXML{},
	}

	// TODO(jfd): if the path we list here is taken from the ListRecycle request we rely on the gateway to prefix it with the mount point

	t := utils.TSToTime(item.DeletionTime).UTC()
	dTime := t.Format(time.RFC1123Z)

	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties
		response.Propstat = append(response.Propstat, propfind.PropstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*props.PropertyXML{},
		})
		// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, props.NewProp("oc:trashbin-original-filename", path.Base(item.Ref.Path)))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, props.NewProp("oc:trashbin-original-location", strings.TrimPrefix(item.Ref.Path, "/")))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, props.NewProp("oc:trashbin-delete-timestamp", strconv.FormatUint(item.DeletionTime.Seconds, 10)))
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, props.NewProp("oc:trashbin-delete-datetime", dTime))
		if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, props.NewPropRaw("d:resourcetype", "<d:collection/>"))
			// TODO(jfd): decide if we can and want to list oc:size for folders
		} else {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				props.NewProp("d:resourcetype", ""),
				props.NewProp("d:getcontentlength", fmt.Sprintf("%d", item.Size)),
			)
		}

	} else {
		// otherwise return only the requested properties
		propstatOK := propfind.PropstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*props.PropertyXML{},
		}
		propstatNotFound := propfind.PropstatXML{
			Status: "HTTP/1.1 404 Not Found",
			Prop:   []*props.PropertyXML{},
		}
		size := fmt.Sprintf("%d", item.Size)
		for i := range pf.Prop {
			switch pf.Prop[i].Space {
			case net.NsOwncloud:
				switch pf.Prop[i].Local {
				case "oc:size":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontentlength", size))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:size", ""))
					}
				case "trashbin-original-filename":
					// yes this is redundant, can be derived from oc:trashbin-original-location which contains the full path, clients should not fetch it
					propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:trashbin-original-filename", path.Base(item.Ref.Path)))
				case "trashbin-original-location":
					// TODO (jfd) double check and clarify the cs3 spec what the Key is about and if Path is only the folder that contains the file or if it includes the filename
					propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:trashbin-original-location", strings.TrimPrefix(item.Ref.Path, "/")))
				case "trashbin-delete-datetime":
					propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:trashbin-delete-datetime", dTime))
				case "trashbin-delete-timestamp":
					propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:trashbin-delete-timestamp", strconv.FormatUint(item.DeletionTime.Seconds, 10)))
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:"+pf.Prop[i].Local, ""))
				}
			case net.NsDav:
				switch pf.Prop[i].Local {
				case "getcontentlength":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getcontentlength", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontentlength", size))
					}
				case "resourcetype":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype":
					if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontenttype", "httpd/unix-directory"))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getcontenttype", ""))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:"+pf.Prop[i].Local, ""))
				}
			default:
				// TODO (jfd) lookup shortname for unknown namespaces?
				propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp(pf.Prop[i].Space+":"+pf.Prop[i].Local, ""))
			}
		}
		response.Propstat = append(response.Propstat, propstatOK, propstatNotFound)
	}

	return &response, nil
}

func (h *TrashbinHandler) restore(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, basePath, dst, key, itemPath string) {
	ctx, span := rtrace.Provider.Tracer("trash-bin").Start(r.Context(), "restore")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Logger()

	overwrite := r.Header.Get(net.HeaderOverwrite)

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	space, rpcstatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, dst)
	if err != nil {
		sublog.Error().Err(err).Str("path", basePath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rpcstatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcstatus)
		return
	}
	dstRef := spacelookup.MakeRelativeReference(space, dst, false)

	dstStatReq := &provider.StatRequest{
		Ref: dstRef,
	}

	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		errors.HandleErrorStatus(&sublog, w, dstStatRes.Status)
		return
	}

	// Restoring to a non-existent location is not supported by the WebDAV spec. The following block ensures the target
	// restore location exists, and if it doesn't returns a conflict error code.
	if dstStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND && isNested(dst) {
		parentStatReq := &provider.StatRequest{
			Ref: spacelookup.MakeRelativeReference(space, filepath.Dir(dst), false),
		}

		parentStatResponse, err := client.Stat(ctx, parentStatReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if parentStatResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			errors.HandleErrorStatus(&sublog, w, &rpc.Status{Code: rpc.Code_CODE_FAILED_PRECONDITION})
			return
		}
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if overwrite != "T" {
			sublog.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			b, err := errors.Marshal(
				http.StatusPreconditionFailed,
				"The destination node already exists, and the overwrite header is set to false",
				net.HeaderOverwrite,
			)
			errors.HandleWebdavError(&sublog, w, b, err)
			return
		}
		// delete existing tree
		delReq := &provider.DeleteRequest{Ref: dstRef}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc delete request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			errors.HandleErrorStatus(&sublog, w, delRes.Status)
			return
		}
	}

	sourceSpace, rpcstatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, basePath)
	if err != nil {
		sublog.Error().Err(err).Str("path", basePath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rpcstatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcstatus)
		return
	}
	req := &provider.RestoreRecycleItemRequest{
		// use the target path to find the storage provider
		// this means we can only undelete on the same storage, not to a different folder
		// use the key which is prefixed with the StoragePath to lookup the correct storage ...
		// TODO currently limited to the home storage
		Ref:        spacelookup.MakeRelativeReference(sourceSpace, basePath, false),
		Key:        path.Join(key, itemPath),
		RestoreRef: dstRef,
	}

	res, err := client.RestoreRecycleItem(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc restore recycle item request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			w.WriteHeader(http.StatusForbidden)
			b, err := errors.Marshal(http.StatusForbidden, "Permission denied to restore", "")
			errors.HandleWebdavError(&sublog, w, b, err)
		}
		errors.HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, dstStatRes.Status)
		return
	}

	info := dstStatRes.Info
	w.Header().Set(net.HeaderContentType, info.MimeType)
	w.Header().Set(net.HeaderETag, info.Etag)
	w.Header().Set(net.HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(info.Id))
	w.Header().Set(net.HeaderOCETag, info.Etag)

	w.WriteHeader(successCode)
}

// delete has only a key
func (h *TrashbinHandler) delete(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, basePath, key, itemPath string) {
	ctx, span := rtrace.Provider.Tracer("trash-bin").Start(r.Context(), "erase")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("key", key).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// set key as opaque id, the storageprovider will use it as the key for the
	// storage drives  PurgeRecycleItem key call
	space, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, basePath)
	if err != nil {
		sublog.Error().Err(err).Str("path", basePath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	req := &provider.PurgeRecycleRequest{
		Ref: spacelookup.MakeRelativeReference(space, basePath, false),
		Key: path.Join(key, itemPath),
	}

	res, err := client.PurgeRecycle(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc restore recycle item request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusNoContent)
	case rpc.Code_CODE_NOT_FOUND:
		sublog.Debug().Str("path", basePath).Str("key", key).Interface("status", res.Status).Msg("resource not found")
		w.WriteHeader(http.StatusConflict)
		m := fmt.Sprintf("path %s not found", basePath)
		b, err := errors.Marshal(http.StatusConflict, m, "")
		errors.HandleWebdavError(&sublog, w, b, err)
	case rpc.Code_CODE_PERMISSION_DENIED:
		w.WriteHeader(http.StatusForbidden)
		var m string
		if key == "" {
			m = "Permission denied to purge recycle"
		} else {
			m = "Permission denied to delete"
		}
		b, err := errors.Marshal(http.StatusForbidden, m, "")
		errors.HandleWebdavError(&sublog, w, b, err)
	default:
		errors.HandleErrorStatus(&sublog, w, res.Status)
	}
}

func isNested(p string) bool {
	dir, _ := path.Split(p)
	return dir != "/"
}
