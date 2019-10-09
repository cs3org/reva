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

package datagatewaysvc

import (
	"io"
	"net/http"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/utils"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("datagatewaysvc", New)
}

type config struct {
	Prefix          string `mapstructure:"prefix"`
	GatewayEndpoint string `mapstructure:"gatewaysvc"`
}

type svc struct {
	conf    *config
	handler http.Handler
}

// New returns a new datagatewaysvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	s := &svc{conf: conf}
	s.setHandler()
	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "HEAD":
			addCorsHeader(w)
			w.WriteHeader(http.StatusOK)
			return
		case "GET":
			s.doGet(w, r)
			return
		case "PUT":
			s.doPut(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	})
}

func addCorsHeader(res http.ResponseWriter) {
	headers := res.Header()
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Headers", "Content-Type, Origin, Authorization")
	headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
}

func (s *svc) doGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := r.URL.Path
	ref := &storageproviderv0alphapb.Reference{Spec: &storageproviderv0alphapb.Reference_Path{Path: fn}}

	c, err := pool.GetGatewayServiceClient(s.conf.GatewayEndpoint)
	if err != nil {
		log.Err(err).Msg("error getting gateway service client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.InitiateFileDownloadRequest{
		Ref: ref,
	}

	res, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		log.Err(err).Msg("error calling InitiateFileDownload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		log.Error().Str("code", res.Status.Code.String()).Msg("wrong response from calling InitiateFileDownload")
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// we call the endpoint and we pipe the response body
	endpoint := res.DownloadEndpoint
	httpClient := utils.GetHTTPClient(ctx)
	httpReq, err := utils.NewRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		log.Err(err).Msg("wrong request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Err(err).Msg("error doing GET request to data service")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if httpRes.StatusCode != http.StatusOK {
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	defer httpRes.Body.Close()
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, httpRes.Body)
	if err != nil {
		log.Err(err).Msg("error writing body after header were set")
	}
}

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := r.URL.Path
	ref := &storageproviderv0alphapb.Reference{Spec: &storageproviderv0alphapb.Reference_Path{Path: fn}}

	c, err := pool.GetGatewayServiceClient(s.conf.GatewayEndpoint)
	if err != nil {
		log.Err(err).Msg("error getting gateway service client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.InitiateFileUploadRequest{
		Ref: ref,
	}

	res, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		log.Err(err).Msg("error calling InitiateFileUpload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		log.Error().Str("code", res.Status.Code.String()).Msg("wrong response from calling InitiateFileUpload")
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// we call the endpoint and we pipe the response body
	endpoint := res.UploadEndpoint
	httpClient := utils.GetHTTPClient(ctx)
	httpReq, err := utils.NewRequest(ctx, "PUT", endpoint, r.Body)
	if err != nil {
		log.Err(err).Msg("wrong request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Err(err).Msg("error doing PUT request to data service")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer httpRes.Body.Close()

	w.WriteHeader(httpRes.StatusCode)
}
