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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/storagespace"
)

// PostprocessingHandler offers option to manually start postprocessing steps
type PostprocessingHandler struct {
}

func (h *PostprocessingHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
func (h *PostprocessingHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		typ, rest := router.ShiftPath(r.URL.Path)
		id, _ := router.ShiftPath(rest)
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

		switch typ {
		default:
			w.WriteHeader(http.StatusNotFound)
		case "virusscan":
			h.handleVirusScan(w, r, s, &did)
		}

	})
}

func (h *PostprocessingHandler) handleVirusScan(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	if err := h.doVirusScan(ctx, s.gwClient, rid, r.Header.Get("x-access-token"), s.stream); err != nil {
		log.Error().Err(err).Interface("resourceid", rid).Msg("cannot do virus scan")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *PostprocessingHandler) doVirusScan(ctx context.Context, client gateway.GatewayAPIClient, rid *provider.ResourceId, revatoken string, pub events.Publisher) error {
	ref := &provider.Reference{ResourceId: rid, Path: "."}

	dRes, err := client.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{Ref: ref})
	if err != nil {
		return err
	}

	if code := dRes.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return fmt.Errorf("Unexpected status code from InitiateFileDownload: %v %v", code, dRes.GetStatus().GetMessage())
	}

	var downloadEP, downloadToken string
	for _, p := range dRes.Protocols {
		if p.Protocol == "spaces" {
			downloadEP, downloadToken = p.DownloadEndpoint, p.Token
		}
	}

	// we need to add the filename for other services
	// or do we? Other services could stat themselves...
	var filename string
	var filesize uint64
	if res, err := client.Stat(ctx, &provider.StatRequest{Ref: ref}); err == nil && res.GetStatus().GetCode() == rpc.Code_CODE_OK {
		filename = res.GetInfo().GetName()
		filesize = res.GetInfo().GetSize()
	}

	return events.Publish(pub, events.StartPostprocessingStep{
		StepToStart:   events.PPStepAntivirus,
		URL:           downloadEP,
		Token:         downloadToken,
		ResourceID:    rid,
		Filesize:      filesize,
		RevaToken:     revatoken,
		ExecutingUser: ctxpkg.ContextMustGetUser(ctx),
		Filename:      filename,
	})
}
