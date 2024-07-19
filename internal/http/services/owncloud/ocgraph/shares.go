// Copyright 2018-2024 CERN
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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/spaces/

package ocgraph

import (
	"encoding/json"
	"net/http"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

func (s *svc) getSharedWithMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resShares, err := gw.ListExistingReceivedShares(ctx, &collaborationv1beta1.ListReceivedSharesRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error getting received shares")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	shares := make([]*libregraph.DriveItem, 0, len(resShares.Shares))
	for _, s := range resShares.Shares {
		shares = append(shares, cs3ReceivedShareToDriveItem(s))
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shares,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling shares as json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func cs3ReceivedShareToDriveItem(share *gateway.SharedResourceInfo) *libregraph.DriveItem {
	return &libregraph.DriveItem{
		UIHidden:          libregraph.PtrBool(share.Share.Hidden),
		ClientSynchronize: libregraph.PtrBool(true),
		CreatedBy: &libregraph.IdentitySet{
			User: &libregraph.Identity{
				DisplayName: "", // TODO: understand if needed, in case needs to be resolved
				Id:          &share.Share.Share.Creator.OpaqueId,
			},
		},
		ETag: &share.ResourceInfo.Etag,
		File: &libregraph.OpenGraphFile{
			MimeType: &share.ResourceInfo.MimeType,
		},
		Id:                   libregraph.PtrString(libregraphShareID(share.Share.Share.Id)),
		LastModifiedDateTime: libregraph.PtrTime(time.Unix(int64(share.ResourceInfo.Mtime.Seconds), int64(share.ResourceInfo.Mtime.Nanos))),
		Name:                 libregraph.PtrString(share.ResourceInfo.Name),
		ParentReference:      &libregraph.ItemReference{}, // TODO: do we have enough info?
		RemoteItem: &libregraph.RemoteItem{
			CreatedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					DisplayName: "", // TODO: understand if needed, in case needs to be resolved
					Id:          &share.Share.Share.Creator.OpaqueId,
				},
			},
			ETag: &share.ResourceInfo.Etag,
			File: &libregraph.OpenGraphFile{
				MimeType: &share.ResourceInfo.MimeType,
			},
			Id:                   nil, // TODO: space id of the resource
			LastModifiedDateTime: libregraph.PtrTime(time.Unix(int64(share.ResourceInfo.Mtime.Seconds), int64(share.ResourceInfo.Mtime.Nanos))),
			Name:                 libregraph.PtrString(share.ResourceInfo.Name),
			ParentReference:      &libregraph.ItemReference{}, // TODO: space id of the resource
			Permissions:          []libregraph.Permission{
				// TODO
			},
			Size: libregraph.PtrInt64(int64(share.ResourceInfo.Size)),
		},
		Size: libregraph.PtrInt64(int64(share.ResourceInfo.Size)),
	}
}
