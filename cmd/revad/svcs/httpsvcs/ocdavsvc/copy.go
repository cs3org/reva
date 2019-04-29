// Copyright 2018-2019 CERN
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

package ocdavsvc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cernbox/reva/pkg/appctx"
	"github.com/cernbox/reva/pkg/token"
)

func (s *svc) doCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	src := r.URL.Path
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")

	log.Info().Str("source", src).Str("destination", dstHeader).Str("overwrite", overwrite).Msg("copy")

	if dstHeader == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// strip baseURL from destination
	dstURL, err := url.ParseRequestURI(dstHeader)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	urlPath := dstURL.Path
	baseURI := r.Context().Value(ctxKeyBaseURI).(string)
	log.Info().Str("url-path", urlPath).Str("base-uri", baseURI).Msg("copy")
	i := strings.Index(urlPath, baseURI)
	if i == -1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// check src exists
	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: src},
	}
	srcStatReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if srcStatRes.Status.Code != rpcpb.Code_CODE_OK {
		if srcStatRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO check if path is on same storage, return 502 on problems, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	dst := path.Clean(urlPath[len(baseURI):])

	// check dst exists
	ref = &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
	}
	dstStatReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var successCode int
	if dstStatRes.Status.Code == rpcpb.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			log.Warn().Str("dst", dst).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}

	} else {
		successCode = http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref = &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusConflict) // 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}
		// TODO what if intermediate is a file?
	}

	err = descend(ctx, client, srcStatRes.Info, dst)
	if err != nil {
		log.Error().Err(err).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func descend(ctx context.Context, client storageproviderv0alphapb.StorageProviderServiceClient, src *storageproviderv0alphapb.ResourceInfo, dst string) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Str("dst", dst).Msg("descending")
	if src.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &storageproviderv0alphapb.CreateContainerRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
			},
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil || createRes.Status.Code != rpcpb.Code_CODE_OK {
			return err
		}

		// descend for children
		listReq := &storageproviderv0alphapb.ListContainerRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{Path: src.Path},
			},
		}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			return fmt.Errorf("status code %d", res.Status.Code)
		}

		for _, e := range res.Infos {
			childDst := path.Join(dst, path.Base(e.Path))
			err := descend(ctx, client, e, childDst)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		//TODO: make header / auth configurable, check if token is available before doing stat requests
		tkn, ok := token.ContextGetToken(ctx)
		if !ok {
			return fmt.Errorf("could not read token from context")
		}

		// 1. get download url
		dReq := &storageproviderv0alphapb.InitiateFileDownloadRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{Path: src.Path},
			},
		}

		dRes, err := client.InitiateFileDownload(ctx, dReq)
		if err != nil {
			return err
		}

		if dRes.Status.Code != rpcpb.Code_CODE_OK {
			return fmt.Errorf("status code %d", dRes.Status.Code)
		}

		// 2. get upload url

		uReq := &storageproviderv0alphapb.InitiateFileUploadRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpcpb.Code_CODE_OK {
			return fmt.Errorf("status code %d", uRes.Status.Code)
		}

		// 3. do download

		httpDownloadReq, err := http.NewRequest("GET", dRes.DownloadEndpoint, nil)
		if err != nil {
			return err
		}

		httpDownloadReq.Header.Set("X-Access-Token", tkn)

		// TODO(labkode): harden http client
		// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		httpDownloadClient := &http.Client{
			Timeout: time.Second * 10,
		}

		httpDownloadRes, err := httpDownloadClient.Do(httpDownloadReq)
		if err != nil {
			return err
		}

		if httpDownloadRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

		// do upload
		// TODO(jfd): check if large files are really streamed

		httpUploadReq, err := http.NewRequest("PUT", uRes.UploadEndpoint, httpDownloadRes.Body)
		if err != nil {
			return err
		}

		httpUploadReq.Header.Set("X-Access-Token", tkn)

		// TODO(labkode): harden http client
		// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		httpUploadClient := &http.Client{
			Timeout: time.Second * 10,
		}

		httpRes, err := httpUploadClient.Do(httpUploadReq)
		if err != nil {
			return err
		}

		if httpRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

	}
	return nil
}
