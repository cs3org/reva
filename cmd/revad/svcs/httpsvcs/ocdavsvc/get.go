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
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	"github.com/cernbox/reva/pkg/token"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/utils"
)

func (s *svc) doGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	client, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &storageproviderv0alphapb.StatRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
	}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, sRes.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	info := sRes.Info
	if info.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
		logger.Println(ctx, "resource is a folder, cannot be downloaded")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	dReq := &storageproviderv0alphapb.InitiateFileDownloadRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
	}

	dRes, err := client.InitiateFileDownload(ctx, dReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dRes.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, dRes.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dataServerURL := dRes.DownloadEndpoint
	// TODO(labkode): perfrom protocol switch
	httpReq, err := http.NewRequest("GET", dataServerURL, nil)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	//TODO: make header / auth configurable, check if token is available before doing stat requests
	tkn, ok := token.ContextGetToken(ctx)
	if !ok {
		logger.Error(ctx, errors.New("could not read token from context"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("X-Access-Token", tkn)

	// TODO(labkode): harden http client
	// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if httpRes.StatusCode != http.StatusOK {
		logger.Println(ctx, httpRes.StatusCode)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", fmt.Sprintf("%s:%s", info.Id.StorageId, info.Id.OpaqueId))
	w.Header().Set("OC-ETag", info.Etag)
	t := utils.TSToTime(info.Mtime)
	lastModifiedString := t.Format(time.RFC1123)
	w.Header().Set("Last-Modified", lastModifiedString)
	/*
		if md.Checksum != "" {
			w.Header().Set("OC-Checksum", md.Checksum)
		}
	*/
	if _, err := io.Copy(w, httpRes.Body); err != nil {
		logger.Error(ctx, err)
	}
}
