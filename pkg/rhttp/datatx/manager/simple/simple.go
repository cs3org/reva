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
	"fmt"
	"io"
	"net/http"
	"strconv"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp/datatx"
	"github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/storage"
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
func New(m map[string]interface{}) (datatx.DataTX, error) {
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
		case "GET":
			var fn string
			files, ok := r.URL.Query()["filename"]
			if !ok || len(files[0]) < 1 {
				fn = r.URL.Path
			} else {
				fn = files[0]
			}

			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fn}}

			// TODO check If-Range condition

			var ranges []datatx.HTTPRange
			var md *provider.ResourceInfo
			var err error
			if r.Header.Get("Range") != "" {
				md, err = fs.GetMD(ctx, ref, nil)
				switch err.(type) {
				case nil:
					ranges, err = datatx.ParseRange(r.Header.Get("Range"), int64(md.Size))
					if err != nil || len(ranges) > 1 { // we currently only support one range
						if err == datatx.ErrNoOverlap {
							w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", md.Size))
						}
						w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
						fmt.Fprintln(w, err)
						return
					}
					w.Header().Set("Content-Range", datatx.FormatRange(ranges[0], md.Size))
				case errtypes.IsNotFound:
					sublog.Debug().Err(err).Msg("datasvc: file not found")
					w.WriteHeader(http.StatusNotFound)
					return
				case errtypes.IsPermissionDenied:
					sublog.Debug().Err(err).Msg("datasvc: file not found")
					w.WriteHeader(http.StatusForbidden)
					return
				default:
					sublog.Error().Err(err).Msg("datasvc: error downloading file")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			// TODO always do a stat to set a Content-Length header

			rc, err := fs.Download(ctx, ref)
			if err != nil {
				if _, ok := err.(errtypes.IsNotFound); ok {
					sublog.Debug().Err(err).Msg("datasvc: file not found")
					w.WriteHeader(http.StatusNotFound)
				} else {
					sublog.Error().Err(err).Msg("datasvc: error downloading file")
					w.WriteHeader(http.StatusInternalServerError)
				}
				return
			}
			defer rc.Close()

			var c int64

			if len(ranges) > 0 {
				sublog.Debug().Int64("start", ranges[0].Start).Int64("length", ranges[0].Length).Msg("datasvc: range request")
				var s io.Seeker
				if s, ok = rc.(io.Seeker); !ok {
					sublog.Error().Int64("start", ranges[0].Start).Int64("length", ranges[0].Length).Msg("datasvc: ReadCloser is not seekable")
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				if _, err = s.Seek(ranges[0].Start, io.SeekStart); err != nil {
					sublog.Error().Err(err).Int64("start", ranges[0].Start).Int64("length", ranges[0].Length).Msg("datasvc: could not seek for range request")
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				w.Header().Set("Content-Range", datatx.FormatRange(ranges[0], md.Size)) // md cannot be null because we did a stat for the range request
				w.Header().Set("Content-Length", strconv.FormatInt(ranges[0].Length, 10))
				w.WriteHeader(http.StatusPartialContent)
				c, err = io.CopyN(w, rc, ranges[0].Length)
				if ranges[0].Length != c {
					sublog.Error().Int64("range-length", ranges[0].Length).Int64("transferred-bytes", c).Msg("range length vs transferred bytes mismatch")
				}
			} else {
				_, err = io.Copy(w, rc)
				// TODO check we sent the correct number of bytes. The stat info might be out dated. we need to send the If-Match etag in the GET to the datagateway
			}

			if err != nil {
				sublog.Error().Err(err).Msg("error copying data to response")
				return
			}

		case "PUT":
			fn := r.URL.Path
			defer r.Body.Close()

			ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fn}}

			err := fs.Upload(ctx, ref, r.Body)
			if err != nil {
				if _, ok := err.(errtypes.IsPartialContent); ok {
					w.WriteHeader(http.StatusPartialContent)
					return
				}
				sublog.Error().Err(err).Msg("error uploading file")
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
