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
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	rstatus "github.com/cs3org/reva/v2/pkg/rgrpc/status"
	rtrace "github.com/cs3org/reva/v2/pkg/trace"
	"github.com/rs/zerolog"
)

func (s *svc) handlePathMkcol(w http.ResponseWriter, r *http.Request, ns string) (status int, err error) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "mkcol")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)
	for _, r := range nameRules {
		if !r.Test(fn) {
			return http.StatusBadRequest, fmt.Errorf("invalid name rule")
		}
	}
	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	client, err := s.getClient()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// stat requested path to make sure it isn't existing yet
	// NOTE: It could be on another storage provider than the 'parent' of it
	sr, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: fn,
		},
	})
	switch {
	case err != nil:
		return http.StatusInternalServerError, err
	case sr.Status.Code == rpc.Code_CODE_OK:
		// https://www.rfc-editor.org/rfc/rfc4918#section-9.3.1:
		// 405 (Method Not Allowed) - MKCOL can only be executed on an unmapped URL.
		return http.StatusMethodNotAllowed, fmt.Errorf("The resource you tried to create already exists")
	case sr.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return rstatus.HTTPStatusFromCode(sr.Status.Code), errtypes.NewErrtypeFromStatus(sr.Status)
	}

	parentPath := path.Dir(fn)

	space, rpcStatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, parentPath)
	switch {
	case err != nil:
		return http.StatusInternalServerError, err
	case rpcStatus.Code == rpc.Code_CODE_NOT_FOUND:
		// https://www.rfc-editor.org/rfc/rfc4918#section-9.3.1:
		// 409 (Conflict) - A collection cannot be made at the Request-URI until
		// one or more intermediate collections have been created.  The server
		// MUST NOT create those intermediate collections automatically.
		return http.StatusConflict, fmt.Errorf("intermediate collection does not exist")
	case rpcStatus.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(rpcStatus.Code), errtypes.NewErrtypeFromStatus(rpcStatus)
	}

	return s.handleMkcol(ctx, w, r, spacelookup.MakeRelativeReference(space, parentPath, false), spacelookup.MakeRelativeReference(space, fn, false), sublog)
}

func (s *svc) handleSpacesMkCol(w http.ResponseWriter, r *http.Request, spaceID string) (status int, err error) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "spaces_mkcol")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "mkcol").Logger()
	client, err := s.getClient()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	parentRef, rpcStatus, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, path.Dir(r.URL.Path), true)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return rstatus.HTTPStatusFromCode(rpcStatus.Code), errtypes.NewErrtypeFromStatus(rpcStatus)
	}

	childRef, rpcStatus, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, r.URL.Path, true)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return rstatus.HTTPStatusFromCode(rpcStatus.Code), errtypes.NewErrtypeFromStatus(rpcStatus)
	}

	return s.handleMkcol(ctx, w, r, parentRef, childRef, sublog)
}

func (s *svc) handleMkcol(ctx context.Context, w http.ResponseWriter, r *http.Request, parentRef, childRef *provider.Reference, log zerolog.Logger) (status int, err error) {
	if r.Body != http.NoBody {
		// We currently do not support extended mkcol https://datatracker.ietf.org/doc/rfc5689/
		// TODO let clients send a body with properties to set on the new resource
		return http.StatusUnsupportedMediaType, fmt.Errorf("extended-mkcol not supported")
	}

	client, err := s.getClient()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// check if parent exists
	parentStatReq := &provider.StatRequest{Ref: parentRef}
	parentStatRes, err := client.Stat(ctx, parentStatReq)
	switch {
	case err != nil:
		return http.StatusInternalServerError, err
	case parentStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		// https://www.rfc-editor.org/rfc/rfc4918#section-9.3.1:
		// 409 (Conflict) - A collection cannot be made at the Request-URI until
		// one or more intermediate collections have been created.  The server
		// MUST NOT create those intermediate collections automatically.
		return http.StatusConflict, fmt.Errorf("intermediate collection does not exist")
	case parentStatRes.Status.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(parentStatRes.Status.Code), errtypes.NewErrtypeFromStatus(parentStatRes.Status)
	}

	// check if child exists
	// TODO again? we did that already in handlePathMkCol and handleSpacesMkCol
	statReq := &provider.StatRequest{Ref: childRef}
	statRes, err := client.Stat(ctx, statReq)
	switch {
	case err != nil:
		return http.StatusInternalServerError, err
	case statRes.Status.Code == rpc.Code_CODE_OK:
		// https://www.rfc-editor.org/rfc/rfc4918#section-9.3.1:
		// 405 (Method Not Allowed) - MKCOL can only be executed on an unmapped URL.
		return http.StatusMethodNotAllowed, fmt.Errorf("The resource you tried to create already exists")
	case statRes.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return rstatus.HTTPStatusFromCode(statRes.Status.Code), errtypes.NewErrtypeFromStatus(statRes.Status)
	}

	req := &provider.CreateContainerRequest{Ref: childRef}
	res, err := client.CreateContainer(ctx, req)
	switch {
	case err != nil:
		return http.StatusInternalServerError, err
	case res.Status.Code == rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusCreated)
		return 0, nil
	case res.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return http.StatusConflict, fmt.Errorf("intermediate collection does not exist")
	default:
		return rstatus.HTTPStatusFromCode(statRes.Status.Code), errtypes.NewErrtypeFromStatus(statRes.Status)
	}
}
