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
	"net/http"
	"path"
	"path/filepath"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"go.opencensus.io/trace"
)

// SpacesHandler handles trashbin requests
type SpacesHandler struct {
	gatewaySvc string
}

func (h *SpacesHandler) init(c *Config) error {
	h.gatewaySvc = c.GatewaySvc
	return nil
}

// Handler handles requests
func (h *SpacesHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx := r.Context()
		// log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.handleOptions(w, r, "spaces")
			return
		}

		var spaceID string
		spaceID, r.URL.Path = router.ShiftPath(r.URL.Path)

		if spaceID == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Checks on the space Id

		if r.Method == "PROPFIND" {
			s.handleSpacesPropfind(w, r, spaceID)
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	})
}

func (s *svc) handleSpacesPropfind(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()

	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Logger()

	// see https://tools.ietf.org/html/rfc4918#section-9.1
	if depth != "0" && depth != "1" && depth != "infinity" {
		sublog.Debug().Str("depth", depth).Msgf("invalid Depth header value")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// Get the getway client
	gatewayClient, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metadataKeys := []string{}
	if pf.Allprop != nil {
		// TODO this changes the behavior and returns all properties if allprops has been set,
		// but allprops should only return some default properties
		// see https://tools.ietf.org/html/rfc4918#section-9.1
		// the description of arbitrary_metadata_keys in https://cs3org.github.io/cs3apis/#cs3.storage.provider.v1beta1.ListContainerRequest an others may need clarification
		// tracked in https://github.com/cs3org/cs3apis/issues/104
		metadataKeys = append(metadataKeys, "*")
	} else {
		for i := range pf.Prop {
			if requiresExplicitFetching(&pf.Prop[i]) {
				metadataKeys = append(metadataKeys, metadataKeyOf(&pf.Prop[i]))
			}
		}
	}

	// retrieve a specific storage space
	lSSReq := &storageProvider.ListStorageSpacesRequest{
		Filters: []*storageProvider.ListStorageSpacesRequest_Filter{
			{
				Type: storageProvider.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &storageProvider.ListStorageSpacesRequest_Filter_Id{
					Id: &storageProvider.StorageSpaceId{
						OpaqueId: spaceID,
					},
				},
			},
		},
	}

	lSSRes, err := gatewayClient.ListStorageSpaces(ctx, lSSReq)
	if err != nil {
		sublog.Error().Err(err).Interface("req", lSSReq).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if lSSRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, lSSRes.Status)
		return
	}

	if len(lSSRes.StorageSpaces) != 1 {
		sublog.Error().Err(err).Interface("req", lSSReq).Msg("unexpected number of spaces")
		return
	}
	space := lSSRes.StorageSpaces[0]

	// TODO:
	// Use ResourceId to make request to the actual storage provider via the gateway.
	// - Copy  the storageId from the storage space root
	// - set the opaque Id to /storageSpaceId/relativePath in
	// Correct fix would be to add a new Reference to the CS3API
	ref := &storageProvider.Reference{
		Spec: &storageProvider.Reference_Id{
			Id: &storageProvider.ResourceId{
				StorageId: space.Root.StorageId,
				OpaqueId:  filepath.Join("/", space.Root.OpaqueId, r.URL.Path), // FIXME this is a hack to pass storage space id and a relative path to the storage provider
			},
		},
	}

	req := &storageProvider.StatRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: metadataKeys,
	}
	res, err := gatewayClient.Stat(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Interface("req", req).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	parentInfo := res.Info
	resourceInfos := []*storageProvider.ResourceInfo{parentInfo}
	if parentInfo.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER && depth == "1" {
		req := &storageProvider.ListContainerRequest{
			Ref:                   ref,
			ArbitraryMetadataKeys: metadataKeys,
		}
		res, err := gatewayClient.ListContainer(ctx, req)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending list container grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(&sublog, w, res.Status)
			return
		}
		resourceInfos = append(resourceInfos, res.Infos...)
	} else if depth == "infinity" {
		// FIXME: doesn't work cross-storage as the results will have the wrong paths!
		// use a stack to explore sub-containers breadth-first
		stack := []string{parentInfo.Path}
		for len(stack) > 0 {
			// retrieve path on top of stack
			currentPath := stack[len(stack)-1]
			ref = &storageProvider.Reference{
				Spec: &storageProvider.Reference_Path{Path: currentPath},
			}
			req := &storageProvider.ListContainerRequest{
				Ref:                   ref,
				ArbitraryMetadataKeys: metadataKeys,
			}
			res, err := gatewayClient.ListContainer(ctx, req)
			if err != nil {
				sublog.Error().Err(err).Str("path", currentPath).Msg("error sending list container grpc request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				HandleErrorStatus(&sublog, w, res.Status)
				return
			}

			resourceInfos = append(resourceInfos, res.Infos...)

			if depth != "infinity" {
				break
			}

			// TODO: stream response to avoid storing too many results in memory

			stack = stack[:len(stack)-1]

			// check sub-containers in reverse order and add them to the stack
			// the reversed order here will produce a more logical sorting of results
			for i := len(res.Infos) - 1; i >= 0; i-- {
				// for i := range res.Infos {
				if res.Infos[i].Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
					stack = append(stack, res.Infos[i].Path)
				}
			}
		}
	}

	// prefix space id to paths
	for i := range resourceInfos {
		resourceInfos[i].Path = path.Join("/", spaceID, resourceInfos[i].Path)
	}

	propRes, err := s.formatPropfind(ctx, &pf, resourceInfos, "") // no namespace because this is relative to the storage space
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	var disableTus bool
	// let clients know this collection supports tus.io POST requests to start uploads
	if parentInfo.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if parentInfo.Opaque != nil {
			_, disableTus = parentInfo.Opaque.Map["disable_tus"]
		}
		if !disableTus {
			w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Tus-Version", "1.0.0")
			w.Header().Set("Tus-Extension", "creation,creation-with-upload")
		}
	}
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}
