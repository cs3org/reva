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

package simple

import (
	"io"
	"net/http"

	"github.com/pkg/errors"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("simple", New)
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
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
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

			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fn}}

			rc, err := fs.Download(ctx, ref)
			if err != nil {
				if _, ok := err.(errtypes.IsNotFound); ok {
					log.Err(err).Msg("datasvc: file not found")
					w.WriteHeader(http.StatusNotFound)
				} else {
					log.Err(err).Msg("datasvc: error downloading file")
					w.WriteHeader(http.StatusInternalServerError)
				}
				return
			}

			_, err = io.Copy(w, rc)
			if err != nil {
				log.Error().Err(err).Msg("error copying data to response")
				return
			}

		case "PUT":
			ctx := r.Context()
			log := appctx.GetLogger(ctx)
			fn := r.URL.Path
			defer r.Body.Close()

			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fn}}

			err := fs.Upload(ctx, ref, r.Body)
			if err != nil {
				if _, ok := err.(errtypes.IsPartialContent); ok {
					w.WriteHeader(http.StatusPartialContent)
					return
				}
				log.Error().Err(err).Msg("error uploading file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	})
	return h, nil
}
