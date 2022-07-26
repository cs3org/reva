// Copyright 2018-2022 CERN
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
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/tags"
)

// TagHandler handles meta requests
type TagHandler struct {
}

func (h *TagHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
func (h *TagHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		id, _ := router.ShiftPath(r.URL.Path)
		did, err := storagespace.ParseID(id)
		if err != nil {
			logger := appctx.GetLogger(r.Context())
			logger.Debug().Str("prop", net.PropOcMetaPathForUser).Msg("invalid resource id")
			w.WriteHeader(http.StatusBadRequest)
			m := fmt.Sprintf("Invalid resource id %v", id)
			b, err := errors.Marshal(http.StatusBadRequest, m, "")
			errors.HandleWebdavError(logger, w, b, err)
			return
		}

		switch r.Method {
		default:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			h.handleCreateTags(w, r, s, &did)
		case http.MethodDelete:
			h.handleDeleteTags(w, r, s, &did)
		}

	})
}

func (h *TagHandler) handleCreateTags(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	h.modifyTags(w, r, s, rid, func(ts *tags.Tags, newtags string) bool {
		if !ts.AddString(newtags) {
			w.WriteHeader(http.StatusBadRequest)
			log := appctx.GetLogger(r.Context()).With().Interface("resourceid", rid).Logger()
			b, err := errors.Marshal(http.StatusBadRequest, "no new tags in createtagsrequest", "")
			errors.HandleWebdavError(&log, w, b, err)
			return false
		}
		return true
	})
}

func (h *TagHandler) handleDeleteTags(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	h.modifyTags(w, r, s, rid, func(ts *tags.Tags, rmtags string) bool {
		if !ts.RemoveString(rmtags) {
			w.WriteHeader(http.StatusBadRequest)
			log := appctx.GetLogger(r.Context()).With().Interface("resourceid", rid).Logger()
			b, err := errors.Marshal(http.StatusBadRequest, "no tags to delete in deletetagsrequest", "")
			errors.HandleWebdavError(&log, w, b, err)
			return false
		}
		return true
	})
}

// should return true if tags should be persisted
type modifyfunc func(existingTags *tags.Tags, tagsParamater string) bool

func (h *TagHandler) modifyTags(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId, f modifyfunc) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx).With().Interface("resourceid", rid).Logger()

	tgs := strings.ToLower(r.FormValue("tags"))
	if tgs == "" {
		w.WriteHeader(http.StatusBadRequest)
		b, err := errors.Marshal(http.StatusBadRequest, "no tags in createtagsrequest", "")
		errors.HandleWebdavError(&log, w, b, err)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	oldtags, err := getExistingTags(ctx, client, rid)
	if err != nil {
		log.Error().Err(err).Msg("error getting existing tags")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ts := tags.FromString(oldtags)
	if !f(ts, tgs) {
		// header should be written by caller in this case
		return
	}

	_, err = client.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{ResourceId: rid},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"tags": ts.AsString(),
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error setting tags")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getExistingTags(ctx context.Context, client gateway.GatewayAPIClient, rid *provider.ResourceId) (string, error) {
	// check for existing tags - could also be done by the storage provider, but then it has to know about tags!?
	sres, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: rid},
	})
	if err != nil {
		return "", err
	}

	if m := sres.GetInfo().GetArbitraryMetadata().GetMetadata(); m != nil {
		return m["tags"], nil
	}

	return "", nil
}
