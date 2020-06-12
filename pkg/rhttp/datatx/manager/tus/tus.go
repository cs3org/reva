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
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	registry.Register("tus", New)
}

type manager struct {
	proxy *proxy
}

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
	proxy := newProxy(fs)
	composer := tusd.NewStoreComposer()
	proxy.UseIn(composer)

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
		case "POST":
			handler.PostFile(w, r)
		case "HEAD":
			handler.HeadFile(w, r)
		case "PATCH":
			handler.PatchFile(w, r)
		case "DELETE":
			handler.DelFile(w, r)
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
		}
	}))

	return h, nil
}

// Composable is the interface that a struct needs to implement to be composable by this composer
type Composable interface {
	UseIn(composer *tusd.StoreComposer)
}

type proxy struct {
	fs storage.FS
}

func newProxy(fs storage.FS) *proxy {
	return &proxy{fs: fs}
}

func (p *proxy) UseIn(c *tusd.StoreComposer) {
	c.UseCore(p)
	//c.UseTerminater(fs)
	//c.UseConcater(fs)
	//c.UseLengthDeferrer(fs)
}

func (p *proxy) NewUpload(ctx context.Context, info tusd.FileInfo) (upload tusd.Upload, err error) {
	return
}

func (p *proxy) GetUpload(ctx context.Context, id string) (upload tusd.Upload, err error) {
	return nil, nil
}
