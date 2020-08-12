// Copyright 2018-2020 CERN
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

package dataprovider

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/token"
	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"
)

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := r.URL.Path

	fsfn := strings.TrimPrefix(fn, s.conf.Prefix)
	ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fsfn}}

	err := s.storage.Upload(ctx, ref, r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error uploading file")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	r.Body.Close()
	w.WriteHeader(http.StatusOK)
}

func (s *svc) doTusPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	fp := r.Header.Get("File-Path")
	if fp == "" {
		log.Error().Msg("File-Path header not present")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	length, err := strconv.ParseInt(r.Header.Get("File-Size"), 10, 64)
	if err != nil {
		log.Error().Msg("File-Size header not present")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dataServerURL := fmt.Sprintf("http://%s%s", r.Host, r.RequestURI)

	// create the tus client.
	c := tus.DefaultConfig()
	c.Resume = true
	c.HttpClient = rhttp.GetHTTPClient(
		rhttp.Context(ctx),
		rhttp.Timeout(time.Duration(s.conf.Timeout*int64(time.Second))),
		rhttp.Insecure(s.conf.Insecure),
	)
	c.Store, err = memorystore.NewMemoryStore()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	c.Header.Set(token.TokenHeader, token.ContextMustGetToken(ctx))

	tusc, err := tus.NewClient(dataServerURL, c)
	if err != nil {
		log.Error().Err(err).Msg("error starting TUS client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metadata := map[string]string{
		"filename": path.Base(fp),
		"dir":      path.Dir(fp),
	}

	upload := tus.NewUpload(r.Body, length, metadata, "")
	defer r.Body.Close()

	// create the uploader.
	c.Store.Set(upload.Fingerprint, dataServerURL)
	uploader := tus.NewUploader(tusc, dataServerURL, upload, 0)

	// start the uploading process.
	err = uploader.Upload()
	if err != nil {
		log.Error().Err(err).Msg("Could not start TUS upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
