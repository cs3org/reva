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
	"strconv"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opencensus.io/trace"
)

var (
	errInvalidValue = errors.New("invalid value")
)

func (s *svc) handlePathCopy(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	src := path.Join(ns, r.URL.Path)

	dst, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dst = path.Join(ns, dst)

	sublog := appctx.GetLogger(ctx).With().Str("src", src).Str("dst", dst).Logger()

	srcRef := &provider.Reference{Path: src}

	// check dst exists
	dstRef := &provider.Reference{Path: dst}

	intermediateDirRefFunc := func() (*provider.Reference, *rpc.Status, error) {
		intermediateDir := path.Dir(dst)
		ref := &provider.Reference{Path: intermediateDir}
		return ref, &rpc.Status{Code: rpc.Code_CODE_OK}, nil
	}

	srcInfo, depth, successCode, ok := s.prepareCopy(ctx, w, r, srcRef, dstRef, intermediateDirRefFunc, sublog)
	if !ok {
		return
	}
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = s.executePathCopy(ctx, client, srcInfo, dst, depth == "infinity")
	if err != nil {
		sublog.Error().Err(err).Str("depth", depth).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func (s *svc) executePathCopy(ctx context.Context, client gateway.GatewayAPIClient, src *provider.ResourceInfo, dst string, recurse bool) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Str("dst", dst).Msg("descending")
	if src.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: &provider.Reference{Path: dst},
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
			Ref: &provider.Reference{Path: src.Path},
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
			err := s.executePathCopy(ctx, client, res.Infos[i], childDst, recurse)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url

		dReq := &provider.InitiateFileDownloadRequest{
			Ref: &provider.Reference{Path: src.Path},
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
			Ref: &provider.Reference{Path: dst},
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(strconv.FormatUint(src.GetSize(), 10)),
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

func (s *svc) handleSpacesCopy(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	dst, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()

	// retrieve a specific storage space
	srcRef, status, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	// retrieve a specific storage space
	dstRef, status, err := s.lookUpStorageSpaceReference(ctx, dstSpaceID, dstRelPath)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	intermediateDirRefFunc := func() (*provider.Reference, *rpc.Status, error) {
		intermediateDir := path.Dir(dstRelPath)
		return s.lookUpStorageSpaceReference(ctx, dstSpaceID, intermediateDir)
	}

	srcInfo, depth, successCode, ok := s.prepareCopy(ctx, w, r, srcRef, dstRef, intermediateDirRefFunc, sublog)
	if !ok {
		return
	}
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = s.executeSpacesCopy(ctx, client, srcInfo, dstRef, depth == "infinity")
	if err != nil {
		sublog.Error().Err(err).Str("depth", depth).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func (s *svc) executeSpacesCopy(ctx context.Context, client gateway.GatewayAPIClient, src *provider.ResourceInfo, dst *provider.Reference, recurse bool) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Interface("dst", dst).Msg("descending")

	if src.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: dst,
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
		listReq := &provider.ListContainerRequest{Ref: &provider.Reference{ResourceId: src.Id}}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", res.Status.Code)
		}

		for i := range res.Infos {
			childPath := strings.TrimPrefix(res.Infos[i].Path, src.Path)
			childRef := &provider.Reference{
				ResourceId: src.Id,
				Path:       path.Join(dst.Path, childPath),
			}
			err := s.executeSpacesCopy(ctx, client, res.Infos[i], childRef, recurse)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url

		dReq := &provider.InitiateFileDownloadRequest{Ref: &provider.Reference{ResourceId: src.Id}}

		dRes, err := client.InitiateFileDownload(ctx, dReq)
		if err != nil {
			return err
		}

		if dRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", dRes.Status.Code)
		}

		var downloadEP, downloadToken string
		for _, p := range dRes.Protocols {
			if p.Protocol == "spaces" {
				downloadEP, downloadToken = p.DownloadEndpoint, p.Token
			}
		}

		// 2. get upload url

		uReq := &provider.InitiateFileUploadRequest{
			Ref: dst,
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(strconv.FormatUint(src.GetSize(), 10)),
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

func (s *svc) prepareCopy(ctx context.Context, w http.ResponseWriter, r *http.Request, srcRef, dstRef *provider.Reference, intermediateDirRef func() (*provider.Reference, *rpc.Status, error), log zerolog.Logger) (*provider.ResourceInfo, string, int, bool) {
	overwrite, err := extractOverwrite(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, "", 0, false
	}
	depth, err := extractDepth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, "", 0, false
	}

	log.Debug().Str("overwrite", overwrite).Str("depth", depth).Msg("copy")

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return nil, "", 0, false
	}

	srcStatReq := &provider.StatRequest{Ref: srcRef}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return nil, "", 0, false
	}

	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&log, w, srcStatRes.Status)
		return nil, "", 0, false
	}

	dstStatReq := &provider.StatRequest{Ref: dstRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return nil, "", 0, false
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&log, w, srcStatRes.Status)
		return nil, "", 0, false
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			log.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return nil, "", 0, false
		}

	} else {
		// check if an intermediate path / the parent exists
		intermediateRef, status, err := intermediateDirRef()
		if err != nil {
			log.Error().Err(err).Msg("error sending a grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil, "", 0, false
		}

		if status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(&log, w, status)
			return nil, "", 0, false
		}
		intStatReq := &provider.StatRequest{Ref: intermediateRef}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil, "", 0, false
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				log.Debug().Interface("parent", intermediateRef).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				HandleErrorStatus(&log, w, srcStatRes.Status)
			}
			return nil, "", 0, false
		}
		// TODO what if intermediate is a file?
	}

	return srcStatRes.Info, depth, successCode, true
}

func extractOverwrite(w http.ResponseWriter, r *http.Request) (string, error) {
	overwrite := r.Header.Get(HeaderOverwrite)
	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		return "", errInvalidValue
	}

	return overwrite, nil
}

func extractDepth(w http.ResponseWriter, r *http.Request) (string, error) {
	depth := r.Header.Get(HeaderDepth)
	if depth == "" {
		depth = "infinity"
	}
	if depth != "infinity" && depth != "0" {
		return "", errInvalidValue
	}
	return depth, nil
}
