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
	"path/filepath"
	"strconv"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

type copy struct {
	source      *provider.Reference
	sourceInfo  *provider.ResourceInfo
	destination *provider.Reference
	depth       net.Depth
	successCode int
}

func (s *svc) handlePathCopy(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "copy")
	defer span.End()

	src := path.Join(ns, r.URL.Path)
	dst, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	for _, r := range nameRules {
		if !r.Test(dst) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	dst = path.Join(ns, dst)

	sublog := appctx.GetLogger(ctx).With().Str("src", src).Str("dst", dst).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	srcSpace, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, src)
	if err != nil {
		sublog.Error().Err(err).Str("path", src).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}
	dstSpace, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, dst)
	if err != nil {
		sublog.Error().Err(err).Str("path", dst).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	cp := s.prepareCopy(ctx, w, r, spacelookup.MakeRelativeReference(srcSpace, src, false), spacelookup.MakeRelativeReference(dstSpace, dst, false), &sublog)
	if cp == nil {
		return
	}

	if err := s.executePathCopy(ctx, client, w, r, cp); err != nil {
		sublog.Error().Err(err).Str("depth", cp.depth.String()).Msg("error executing path copy")
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
				b, err := errors.Marshal(http.StatusForbidden, m, "")
				errors.HandleWebdavError(log, w, b, err)
			}
			return nil
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if cp.depth != net.DepthInfinity {
			return nil
		}

		// descend for children
		listReq := &provider.ListContainerRequest{
			Ref: cp.source,
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
			child := filepath.Base(res.Infos[i].Path)
			src := &provider.Reference{
				ResourceId: cp.source.ResourceId,
				Path:       utils.MakeRelativePath(filepath.Join(cp.source.Path, child)),
			}
			childDst := &provider.Reference{
				ResourceId: cp.destination.ResourceId,
				Path:       utils.MakeRelativePath(filepath.Join(cp.destination.Path, child)),
			}
			err := s.executePathCopy(ctx, client, w, r, &copy{source: src, sourceInfo: res.Infos[i], destination: childDst, depth: cp.depth, successCode: cp.successCode})
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url

		dReq := &provider.InitiateFileDownloadRequest{
			Ref: cp.source,
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
			if p.Protocol == "spaces" {
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
				b, err := errors.Marshal(http.StatusForbidden, m, "")
				errors.HandleWebdavError(log, w, b, err)
				return nil
			}
			errors.HandleErrorStatus(log, w, uRes.Status)
			return nil
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

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// retrieve a specific storage space
	srcRef, status, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, r.URL.Path, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	// retrieve a specific storage space
	dstRef, status, err := spacelookup.LookUpStorageSpaceReference(ctx, client, dstSpaceID, dstRelPath, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	cp := s.prepareCopy(ctx, w, r, srcRef, dstRef, &sublog)
	if cp == nil {
		return
	}

	err = s.executeSpacesCopy(ctx, w, client, cp)
	if err != nil {
		sublog.Error().Err(err).Str("depth", cp.depth.String()).Msg("error descending directory")
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
				b, err := errors.Marshal(http.StatusForbidden, m, "")
				errors.HandleWebdavError(log, w, b, err)
			}
			return nil
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if cp.depth != net.DepthInfinity {
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
					net.HeaderUploadLength: {
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
				b, err := errors.Marshal(http.StatusForbidden, m, "")
				errors.HandleWebdavError(log, w, b, err)
				return nil
			}
			errors.HandleErrorStatus(log, w, uRes.Status)
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

func (s *svc) prepareCopy(ctx context.Context, w http.ResponseWriter, r *http.Request, srcRef, dstRef *provider.Reference, log *zerolog.Logger) *copy {
	overwrite, err := extractOverwrite(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		m := fmt.Sprintf("Overwrite header is set to incorrect value %v", overwrite)
		b, err := errors.Marshal(http.StatusBadRequest, m, "")
		errors.HandleWebdavError(log, w, b, err)
		return nil
	}
	dh := r.Header.Get(net.HeaderDepth)
	depth, err := net.ParseDepth(dh)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		m := fmt.Sprintf("Depth header is set to incorrect value %v", dh)
		b, err := errors.Marshal(http.StatusBadRequest, m, "")
		errors.HandleWebdavError(log, w, b, err)
		return nil
	}
	if dh == "" {
		// net.ParseDepth returns "1" for an empty value but copy expects "infinity"
		// so we overwrite it here
		depth = net.DepthInfinity
	}

	log.Debug().Str("overwrite", overwrite).Str("depth", depth.String()).Msg("copy")

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
			b, err := errors.Marshal(http.StatusNotFound, m, "")
			errors.HandleWebdavError(log, w, b, err)
		}
		errors.HandleErrorStatus(log, w, srcStatRes.Status)
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
		errors.HandleErrorStatus(log, w, srcStatRes.Status)
		return nil
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			log.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed)
			m := fmt.Sprintf("Could not overwrite Resource %v", dstRef.Path)
			b, err := errors.Marshal(http.StatusPreconditionFailed, m, "")
			errors.HandleWebdavError(log, w, b, err) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
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
			errors.HandleErrorStatus(log, w, delRes.Status)
			return nil
		}
	} else if p := path.Dir(dstRef.Path); p != "" {
		// check if an intermediate path / the parent exists
		pRef := &provider.Reference{
			ResourceId: dstRef.ResourceId,
			Path:       utils.MakeRelativePath(p),
		}
		intStatReq := &provider.StatRequest{Ref: pRef}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				log.Debug().Interface("parent", pRef).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				errors.HandleErrorStatus(log, w, intStatRes.Status)
			}
			return nil
		}
		// TODO what if intermediate is a file?
	}

	return &copy{source: srcRef, sourceInfo: srcStatRes.Info, depth: depth, successCode: successCode, destination: dstRef}
}

func extractOverwrite(w http.ResponseWriter, r *http.Request) (string, error) {
	overwrite := r.Header.Get(net.HeaderOverwrite)
	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		return "", errInvalidValue
	}

	return overwrite, nil
}
