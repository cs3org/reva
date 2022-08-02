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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/utils"
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
			s.handleOptions(w, r)
			return
		}

		var spaceID string
		spaceID, r.URL.Path = router.ShiftPath(r.URL.Path)

		if spaceID == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		switch r.Method {
		case MethodPropfind:
			s.handleSpacesPropfind(w, r, spaceID)
		case MethodProppatch:
			s.handleSpacesProppatch(w, r, spaceID)
		case MethodLock:
			s.handleLock(w, r, spaceID)
		case MethodUnlock:
			s.handleUnlock(w, r, spaceID)
		case MethodMkcol:
			s.handleSpacesMkCol(w, r, spaceID)
		case MethodMove:
			s.handleSpacesMove(w, r, spaceID)
		case MethodCopy:
			s.handleSpacesCopy(w, r, spaceID)
		case MethodReport:
			s.handleReport(w, r, spaceID)
		case http.MethodGet:
			s.handleSpacesGet(w, r, spaceID)
		case http.MethodPut:
			s.handleSpacesPut(w, r, spaceID)
		case http.MethodPost:
			s.handleSpacesTusPost(w, r, spaceID)
		case http.MethodOptions:
			s.handleOptions(w, r)
		case http.MethodHead:
			s.handleSpacesHead(w, r, spaceID)
		case http.MethodDelete:
			s.handleSpacesDelete(w, r, spaceID)
		default:
			http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		}
	})
}

func (s *svc) lookUpStorageSpaceReference(ctx context.Context, spaceID string, relativePath string) (*storageProvider.Reference, *rpc.Status, error) {
	// Get the getway client
	gatewayClient, err := s.getClient()
	if err != nil {
		return nil, nil, err
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
	if err != nil || lSSRes.Status.Code != rpc.Code_CODE_OK {
		return nil, lSSRes.Status, err
	}

	if len(lSSRes.StorageSpaces) != 1 {
		return nil, nil, fmt.Errorf("unexpected number of spaces")
	}
	space := lSSRes.StorageSpaces[0]

	return &storageProvider.Reference{
		ResourceId: space.Root,
		Path:       utils.MakeRelativePath(relativePath),
	}, lSSRes.Status, nil
}
