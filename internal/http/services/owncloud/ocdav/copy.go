// Copyright 2018-2023 CERN
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
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

type copy struct {
	sourceInfo  *provider.ResourceInfo
	destination *provider.Reference
	depth       string
	successCode int
}

type intermediateDirRefFunc func() (*provider.Reference, *rpc.Status, error)

func (s *svc) handlePathCopy(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "copy")
	defer span.End()

	if s.c.EnableHTTPTpc {
		if r.Header.Get("Source") != "" {
			// HTTP Third-Party Copy Pull mode
			s.handleTPCPull(ctx, w, r, ns)
			return
		} else if r.Header.Get("Destination") != "" {
			// HTTP Third-Party Copy Push mode
			s.handleTPCPush(ctx, w, r, ns)
			return
		}
	}

	// Local copy: in this case Destination is mandatory
	src := path.Join(ns, r.URL.Path)
	dst, err := extractDestination(r)
	if err != nil {
		appctx.GetLogger(ctx).Warn().Msg("HTTP COPY: failed to extract destination")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, r := range nameRules {
		if !r.Test(dst) {
			appctx.GetLogger(ctx).Warn().Msgf("HTTP COPY: destination %s failed validation", dst)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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

	cp := s.prepareCopy(ctx, w, r, srcRef, dstRef, intermediateDirRefFunc, &sublog)
	if cp == nil {
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.executePathCopy(ctx, client, w, r, cp); err != nil {
		sublog.Error().Err(err).Str("depth", cp.depth).Msg("error executing path copy")
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(cp.successCode)
}

func (s *svc) executePathCopy(ctx context.Context, client gateway.GatewayAPIClient, w http.ResponseWriter, r *http.Request, cp *copy) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", cp.sourceInfo.Path).Str("dst", cp.destination.Path).Msg("descending")
	if cp.sourceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: cp.destination,
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil {
			log.Error().Err(err).Msg("error performing create container grpc request")
			return err
		}
		if createRes.Status.Code != rpc.Code_CODE_OK {
			if createRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
				w.WriteHeader(http.StatusForbidden)
				m := fmt.Sprintf("Permission denied to create %v", createReq.Ref.Path)
				b, err := Marshal(exception{
					code:    SabredavPermissionDenied,
					message: m,
				})
				HandleWebdavError(log, w, b, err)
			}
			return nil
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if cp.depth != "infinity" {
			return nil
		}

		// descend for children
		listReq := &provider.ListContainerRequest{
			Ref: &provider.Reference{Path: cp.sourceInfo.Path},
		}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}

		for i := range res.Infos {
			childDst := &provider.Reference{Path: path.Join(cp.destination.Path, path.Base(res.Infos[i].Path))}
			err := s.executePathCopy(ctx, client, w, r, &copy{sourceInfo: res.Infos[i], destination: childDst, depth: cp.depth, successCode: cp.successCode})
			if err != nil {
				return err
			}
		}
	} else {
		// copy file

		// 1. get download url

		dReq := &provider.InitiateFileDownloadRequest{
			Ref: &provider.Reference{Path: cp.sourceInfo.Path},
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
			Ref: cp.destination,
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(strconv.FormatUint(cp.sourceInfo.GetSize(), 10)),
					},
				},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpc.Code_CODE_OK {
			if uRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
				w.WriteHeader(http.StatusForbidden)
				m := fmt.Sprintf("Permissions denied to create %v", uReq.Ref.Path)
				b, err := Marshal(exception{
					code:    SabredavPermissionDenied,
					message: m,
				})
				HandleWebdavError(log, w, b, err)
				return nil
			}
			HandleErrorStatus(log, w, uRes.Status)
			return nil
		}

		var uploadEP, uploadToken string
		for _, p := range uRes.Protocols {
			if p.Protocol == "simple" {
				uploadEP, uploadToken = p.UploadEndpoint, p.Token
			}
		}

		// 3. do download

		httpDownloadReq, err := rhttp.NewRequest(ctx, http.MethodGet, downloadEP, nil)
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

		httpUploadReq, err := rhttp.NewRequest(ctx, http.MethodPut, uploadEP, httpDownloadRes.Body)
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
	return nil
}

func (s *svc) handleSpacesCopy(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "spaces_copy")
	defer span.End()

	dst, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Str("destination", dst).Logger()

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

	dstSpaceID, dstRelPath := rhttp.ShiftPath(dst)

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

	cp := s.prepareCopy(ctx, w, r, srcRef, dstRef, intermediateDirRefFunc, &sublog)
	if cp == nil {
		return
	}
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = s.executeSpacesCopy(ctx, w, client, cp)
	if err != nil {
		sublog.Error().Err(err).Str("depth", cp.depth).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(cp.successCode)
}

func (s *svc) executeSpacesCopy(ctx context.Context, w http.ResponseWriter, client gateway.GatewayAPIClient, cp *copy) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("src", cp.sourceInfo).Interface("dst", cp.destination).Msg("descending")

	if cp.sourceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: cp.destination,
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil {
			log.Error().Err(err).Msg("error performing create container grpc request")
			return err
		}
		if createRes.Status.Code != rpc.Code_CODE_OK {
			if createRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
				w.WriteHeader(http.StatusForbidden)
				// TODO path could be empty or relative...
				m := fmt.Sprintf("Permission denied to create %v", createReq.Ref.Path)
				b, err := Marshal(exception{
					code:    SabredavPermissionDenied,
					message: m,
				})
				HandleWebdavError(log, w, b, err)
			}
			return nil
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if cp.depth != "infinity" {
			return nil
		}

		// descend for children
		listReq := &provider.ListContainerRequest{Ref: &provider.Reference{ResourceId: cp.sourceInfo.Id, Path: "."}}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}

		for i := range res.Infos {
			childRef := &provider.Reference{
				ResourceId: cp.destination.ResourceId,
				Path:       utils.MakeRelativePath(path.Join(cp.destination.Path, res.Infos[i].Path)),
			}
			err := s.executeSpacesCopy(ctx, w, client, &copy{sourceInfo: res.Infos[i], destination: childRef, depth: cp.depth, successCode: cp.successCode})
			if err != nil {
				return err
			}
		}
	} else {
		// copy file
		// 1. get download url
		dReq := &provider.InitiateFileDownloadRequest{Ref: &provider.Reference{ResourceId: cp.sourceInfo.Id, Path: "."}}
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
			Ref: cp.destination,
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					HeaderUploadLength: {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(strconv.FormatUint(cp.sourceInfo.GetSize(), 10)),
					},
				},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpc.Code_CODE_OK {
			if uRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
				w.WriteHeader(http.StatusForbidden)
				// TODO path can be empty or relative
				m := fmt.Sprintf("Permissions denied to create %v", uReq.Ref.Path)
				b, err := Marshal(exception{
					code:    SabredavPermissionDenied,
					message: m,
				})
				HandleWebdavError(log, w, b, err)
				return nil
			}
			HandleErrorStatus(log, w, uRes.Status)
			return nil
		}

		var uploadEP, uploadToken string
		for _, p := range uRes.Protocols {
			if p.Protocol == "simple" {
				uploadEP, uploadToken = p.UploadEndpoint, p.Token
			}
		}

		// 3. do download
		httpDownloadReq, err := rhttp.NewRequest(ctx, http.MethodGet, downloadEP, nil)
		if err != nil {
			return err
		}
		if downloadToken != "" {
			httpDownloadReq.Header.Set(datagateway.TokenTransportHeader, downloadToken)
		}

		httpDownloadRes, err := s.client.Do(httpDownloadReq)
		if err != nil {
			return err
		}
		defer httpDownloadRes.Body.Close()
		if httpDownloadRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

		// 4. do upload

		httpUploadReq, err := rhttp.NewRequest(ctx, http.MethodPut, uploadEP, httpDownloadRes.Body)
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
	return nil
}

func (s *svc) prepareCopy(ctx context.Context, w http.ResponseWriter, r *http.Request, srcRef, dstRef *provider.Reference, intermediateDirRef intermediateDirRefFunc, log *zerolog.Logger) *copy {
	overwrite, err := extractOverwrite(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		m := fmt.Sprintf("Overwrite header is set to incorrect value %v", overwrite)
		b, err := Marshal(exception{
			code:    SabredavBadRequest,
			message: m,
		})
		HandleWebdavError(log, w, b, err)
		return nil
	}
	depth, err := extractDepth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		m := fmt.Sprintf("Depth header is set to incorrect value %v", depth)
		b, err := Marshal(exception{
			code:    SabredavBadRequest,
			message: m,
		})
		HandleWebdavError(log, w, b, err)
		return nil
	}

	log.Debug().Str("overwrite", overwrite).Str("depth", depth).Msg("copy")

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}

	srcStatReq := &provider.StatRequest{Ref: srcRef}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}

	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		if srcStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			m := fmt.Sprintf("Resource %v not found", srcStatReq.Ref.Path)
			b, err := Marshal(exception{
				code:    SabredavNotFound,
				message: m,
			})
			HandleWebdavError(log, w, b, err)
		}
		HandleErrorStatus(log, w, srcStatRes.Status)
		return nil
	}

	dstStatReq := &provider.StatRequest{Ref: dstRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(log, w, srcStatRes.Status)
		return nil
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			log.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed)
			m := fmt.Sprintf("Could not overwrite Resource %v", dstRef.Path)
			b, err := Marshal(exception{
				code:    SabredavPreconditionFailed,
				message: m,
			})
			HandleWebdavError(log, w, b, err) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return nil
		}

		// delete existing tree
		delReq := &provider.DeleteRequest{Ref: dstRef}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc delete request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}

		if delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			HandleErrorStatus(log, w, delRes.Status)
			return nil
		}
	} else {
		// check if an intermediate path / the parent exists
		intermediateRef, status, err := intermediateDirRef()
		if err != nil {
			log.Error().Err(err).Msg("error sending a grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}

		if status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(log, w, status)
			return nil
		}
		intStatReq := &provider.StatRequest{Ref: intermediateRef}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				log.Debug().Interface("parent", intermediateRef).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				HandleErrorStatus(log, w, srcStatRes.Status)
			}
			return nil
		}
		// TODO what if intermediate is a file?
	}

	return &copy{sourceInfo: srcStatRes.Info, depth: depth, successCode: successCode, destination: dstRef}
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
