// Copyright 2018-2024 CERN
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
	"context"
	"net/http"
	"strconv"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rhttp/datatx"
	"github.com/cs3org/reva/v3/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/v3/pkg/rhttp/datatx/utils/download"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
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
func New(ctx context.Context, m map[string]interface{}) (datatx.DataTX, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{conf: c}, nil
}

func (m *manager) Handler(fs storage.FS) (http.Handler, error) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sublog := appctx.GetLogger(ctx).With().Str("datatx", "simple").Logger()

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			download.GetOrHeadFile(w, r, fs, "")
		case http.MethodPut:
			fn := r.URL.Path
			defer r.Body.Close()

			ref := &provider.Reference{Path: fn}
			metadata := map[string]string{}
			if lockid := r.Header.Get(ocdav.HeaderLockID); lockid != "" {
				metadata["lockid"] = lockid
			}
			if lockholder := r.Header.Get(ocdav.HeaderLockHolder); lockholder != "" {
				metadata["lockholder"] = lockholder
			}

			if disableVersioning := r.Header.Get(ocdav.HeaderDisableVersioning); disableVersioning != "" {
				metadata["disableVersioning"] = disableVersioning
			}

			contentLength := r.Header.Get(ocdav.HeaderUploadLength)

			if _, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
				metadata[ocdav.HeaderContentLength] = contentLength
			} else {
				contentLength := r.Header.Get(ocdav.HeaderContentLength)
				if _, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
					metadata[ocdav.HeaderContentLength] = contentLength
				}

			}

			err := fs.Upload(ctx, ref, r.Body, metadata)
			switch v := err.(type) {
			case nil:
				w.WriteHeader(http.StatusOK)
			case errtypes.PartialContent:
				w.WriteHeader(http.StatusPartialContent)
			case errtypes.ChecksumMismatch:
				w.WriteHeader(errtypes.StatusChecksumMismatch)
			case errtypes.NotFound:
				w.WriteHeader(http.StatusNotFound)
			case errtypes.PermissionDenied:
				w.WriteHeader(http.StatusForbidden)
			case errtypes.InvalidCredentials:
				w.WriteHeader(http.StatusUnauthorized)
			case errtypes.InsufficientStorage:
				w.WriteHeader(http.StatusInsufficientStorage)
			case errtypes.Conflict:
				w.WriteHeader(http.StatusConflict)
			default:
				sublog.Error().Err(v).Msg("error uploading file")
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	})
	return h, nil
}
