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

package tus

import (
	"net/http"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp/datatx/utils/download"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
	tusd "github.com/tus/tusd/pkg/handler"
)

func init() {
	registry.Register("tus", New)
}

type config struct{}

type manager struct {
	conf *config
}

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
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{conf: c}, nil
}

func (m *manager) Handler(fs storage.FS) (http.Handler, error) {
	composable, ok := fs.(composable)
	if !ok {
		return nil, errtypes.NotSupported("file system does not support the tus protocol")
	}

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()

	// let the composable storage tell tus which extensions it supports
	composable.UseIn(composer)

	config := tusd.Config{
		StoreComposer: composer,
	}

	handler, err := tusd.NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	h := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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
		case "GET":
			download.GetOrHeadFile(w, r, fs)
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))

	return h, nil
}

// Composable is the interface that a struct needs to implement
// to be composable, so that it can support the TUS methods
type composable interface {
	UseIn(composer *tusd.StoreComposer)
}
