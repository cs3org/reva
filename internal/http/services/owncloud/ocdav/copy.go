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
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"go.opencensus.io/trace"
)

func (s *svc) handleCopy(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	src := path.Join(ns, r.URL.Path)
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}

	dst, err := extractDestination(dstHeader, r.Context().Value(ctxKeyBaseURI).(string))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dst = path.Join(ns, dst)

	sublog := appctx.GetLogger(ctx).With().Str("src", src).Str("dst", dst).Logger()
	sublog.Debug().Str("overwrite", overwrite).Str("depth", depth).Msg("copy")

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if depth != "infinity" && depth != "0" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check src exists
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: src},
	}
	srcStatReq := &provider.StatRequest{Ref: ref}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, srcStatRes.Status)
		return
	}

	// check dst exists
	ref = &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	dstStatReq := &provider.StatRequest{Ref: ref}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&sublog, w, srcStatRes.Status)
		return
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			sublog.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}

	} else {
		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &provider.StatRequest{Ref: ref}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				sublog.Debug().Str("parent", intermediateDir).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				HandleErrorStatus(&sublog, w, srcStatRes.Status)
			}
			return
		}
		// TODO what if intermediate is a file?
	}

	err = s.descend(ctx, client, srcStatRes.Info, dst, depth == "infinity")
	if err != nil {
		sublog.Error().Err(err).Str("depth", depth).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func (s *svc) descend(ctx context.Context, client gateway.GatewayAPIClient, src *provider.ResourceInfo, dst string, recurse bool) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Str("dst", dst).Msg("descending")
	if src.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: dst},
			},
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil || createRes.Status.Code != rpc.Code_CODE_OK {
			return err
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if !recurse {
			return nil
		}

		// descend for children
		listReq := &provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: src.Path},
			},
		}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", res.Status.Code)
		}

		for i := range res.Infos {
			childDst := path.Join(dst, path.Base(res.Infos[i].Path))
			err := s.descend(ctx, client, res.Infos[i], childDst, recurse)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url

		dReq := &provider.InitiateFileDownloadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: src.Path},
			},
		}

		dRes, err := client.InitiateFileDownload(ctx, dReq)
		if err != nil {
			return err
		}

		if dRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", dRes.Status.Code)
		}

		var downloadEP, downloadToken string
		for _, p := range dRes.Protocols {
			if p.Protocol == "simple" {
				downloadEP, downloadToken = p.DownloadEndpoint, p.Token
			}
		}

		// 2. get upload url

		uReq := &provider.InitiateFileUploadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: dst},
			},
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(fmt.Sprintf("%d", src.GetSize())),
					},
				},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", uRes.Status.Code)
		}

		var uploadEP, uploadToken string
		for _, p := range uRes.Protocols {
			if p.Protocol == "simple" {
				uploadEP, uploadToken = p.UploadEndpoint, p.Token
			}
		}

		// 3. do download

		httpDownloadReq, err := rhttp.NewRequest(ctx, "GET", downloadEP, nil)
		if err != nil {
			return err
		}
		httpDownloadReq.Header.Set(datagateway.TokenTransportHeader, downloadToken)

		httpDownloadRes, err := s.client.Do(httpDownloadReq)
		if err != nil {
			return err
		}
		defer httpDownloadRes.Body.Close()
		if httpDownloadRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

		// 4. do upload

		if src.GetSize() > 0 {
			httpUploadReq, err := rhttp.NewRequest(ctx, "PUT", uploadEP, httpDownloadRes.Body)
			if err != nil {
				return err
			}
			httpUploadReq.Header.Set(datagateway.TokenTransportHeader, uploadToken)

			httpUploadRes, err := s.client.Do(httpUploadReq)
			if err != nil {
				return err
			}
			defer httpUploadRes.Body.Close()
			if httpUploadRes.StatusCode != http.StatusOK {
				return err
			}
		}
	}
	return nil
}
