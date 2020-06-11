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

package tus

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/token"
	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"
	"github.com/mitchellh/mapstructure"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	registry.Register("tus", New)
}

type manager struct{}
type config struct{}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a datatx manager implementation that relies on HTTP PUT/GET.
func New(m map[string]interface{}) (datatx.DataTX, error) {
	_, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{}, nil
}

func (m *manager) Handler(prefix string, fs storage.FS) (http.Handler, error) {
	composable, ok := fs.(Composable)
	if !ok {
		// TODO(labkode): break dependency on Go interface and bring TUS logic to CS3APIS for
		// resumable uploads/chunking uploads.
		return nil, fmt.Errorf("dataprovider: configured storage is not tus-compatible")
	}

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()

	// let the composable storage tell tus which extensions it supports
	composable.UseIn(composer)

	config := tusd.Config{
		BasePath:      prefix,
		StoreComposer: composer,
		//Logger:        logger, // TODO use logger
	}

	handler, err := tusd.NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	h := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log := appctx.GetLogger(r.Context())
		log.Info().Msgf("tusd routing: path=%s", r.URL.Path)

		method := r.Method
		// https://github.com/tus/tus-resumable-upload-protocol/blob/master/protocol.md#x-http-method-override
		if r.Header.Get("X-HTTP-Method-Override") != "" {
			method = r.Header.Get("X-HTTP-Method-Override")
		}

		switch method {
		// TODO(pvince81): allow for range-based requests?
		case "GET":
			ctx := r.Context()
			log := appctx.GetLogger(ctx)
			var fn string
			files, ok := r.URL.Query()["filename"]
			if !ok || len(files[0]) < 1 {
				fn = r.URL.Path
			} else {
				fn = files[0]
			}

			fsfn := strings.TrimPrefix(fn, prefix)
			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fsfn}}

			rc, err := fs.Download(ctx, ref)
			if err != nil {
				log.Err(err).Msg("datasvc: error downloading file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			_, err = io.Copy(w, rc)
			if err != nil {
				log.Error().Err(err).Msg("error copying data to response")
				return
			}
			return

		// tus.io based upload

		// uploads are initiated using the CS3 APIs Initiate Download call
		case "POST":
			handler.PostFile(w, r)
		case "HEAD":
			handler.HeadFile(w, r)
		case "PATCH":
			handler.PatchFile(w, r)
		// PUT provides a wrapper around the POST call, to save the caller from
		// the trouble of configuring the tus client.
		case "PUT":
			ctx := r.Context()
			log := appctx.GetLogger(ctx)

			fp := r.Header.Get("File-Path")
			if fp == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			length, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			dataServerURL := fmt.Sprintf("http://%s%s", r.Host, r.RequestURI)

			// create the tus client.
			c := tus.DefaultConfig()
			c.Resume = true
			c.HttpClient = rhttp.GetHTTPClient(ctx)
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

		// TODO Only attach the DELETE handler if the Terminate() method is provided
		case "DELETE":
			handler.DelFile(w, r)
		}
	}))

	return h, nil
}

// Composable is the interface that a struct needs to implement to be composable by this composer
type Composable interface {
	UseIn(composer *tusd.StoreComposer)
}
