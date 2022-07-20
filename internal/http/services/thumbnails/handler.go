// Copyright 2018-2022 CERN
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

package thumbnails

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cs3org/reva/internal/http/services/thumbnails/manager"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("thumbnails", New)
}

type ContextKey int

const (
	ContextKeyPath ContextKey = iota
)

const (
	DefaultWidth  int = 32
	DefaultHeight int = 32
)

type config struct {
	GatewaySVC   string                            `mapstructure:"gateway_svc"`
	Quality      int                               `mapstructure:"quality"`
	Resolutions  []string                          `mapstructure:"quality"`
	Cache        bool                              `mapstructure:"cache"`
	CacheDriver  string                            `mapstructure:"cache_driver"`
	CacheDrivers map[string]map[string]interface{} `mapstructure:"cache_drivers"`
	OutputType   string                            `mapstructure:"output_type"`
	Prefix       string                            `mapstructure:"prefix"`
	Insecure     bool                              `mapstructure:"insecure"`
}

type svc struct {
	c         *config
	router    *chi.Mux
	log       *zerolog.Logger
	thumbnail *manager.Thumbnail
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "thumbnails"
	}
	if c.OutputType == "" {
		c.OutputType = "jpg"
	}
	if c.OutputType == "jpg" && c.Quality == 0 {
		c.Quality = 80
	}
	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

func New(conf map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c := &config{}
	err := mapstructure.Decode(conf, c)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding config")
	}
	c.init()

	r := chi.NewRouter()

	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySVC))
	if err != nil {
		return nil, errors.Wrap(err, "error getting gateway client")
	}

	d := downloader.NewDownloader(gtw, rhttp.Insecure(c.Insecure))

	mgr, err := manager.NewThumbnail(d, &manager.Config{
		Quality:      c.Quality,
		Resolutions:  c.Resolutions,
		Cache:        c.Cache,
		CacheDriver:  c.CacheDriver,
		CacheDrivers: c.CacheDrivers,
	}, log)
	if err != nil {
		return nil, err
	}

	s := &svc{
		c:         c,
		log:       log,
		router:    r,
		thumbnail: mgr,
	}

	// thumbnails for normal files
	r.Group(func(r chi.Router) {
		r.Use(s.DavUserContext)
		r.Get("/files/*", s.Thumbnail)
	})

	// thumbnails for public links
	r.Group(func(r chi.Router) {
		// r.Use(s.DavPublicContext())

		// r.Head("/remote.php/dav/public-files/{token}/*", s.PublicThumbnailHead)
		// r.Head("/dav/public-files/{token}/*", s.PublicThumbnailHead)

		// r.Get("/remote.php/dav/public-files/{token}/*", s.PublicThumbnail)
		// r.Get("/dav/public-files/{token}/*", s.PublicThumbnail)
	})

	return s, nil
}

func (s *svc) DavUserContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		path := chi.URLParam(r, "*")
		path, _ = url.QueryUnescape(path)
		path = "/" + path

		ctx = context.WithValue(ctx, ContextKeyPath, path)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type ThumbnailRequest struct {
	File       string
	Width      int
	Height     int
	OutputType manager.FileType
}

func (s *svc) parseThumbnailRequest(r *http.Request) (*ThumbnailRequest, error) {
	ctx := r.Context()

	file := ctx.Value(ContextKeyPath).(string)
	width, height, err := parseDimensions(r.URL.Query())
	if err != nil {
		return nil, errtypes.BadRequest(fmt.Sprintf("error parsing dimensions: %v", err))
	}

	t := getOutType(s.c.OutputType)

	return &ThumbnailRequest{
		File:       file,
		Width:      width,
		Height:     height,
		OutputType: t,
	}, nil
}

func getOutType(s string) manager.FileType {
	switch s {
	case "bmp":
		return manager.BMPType
	case "png":
		return manager.PNGType
	default:
		return manager.JPEGType
	}
}

func parseDimensions(q url.Values) (int, int, error) {
	width, err := parseDimension(q.Get("x"), "width", DefaultWidth)
	if err != nil {
		return 0, 0, err
	}
	height, err := parseDimension(q.Get("y"), "height", DefaultHeight)
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func parseDimension(d, name string, defaultValue int) (int, error) {
	if d == "" {
		return defaultValue, nil
	}
	result, err := strconv.ParseInt(d, 10, 32)
	if err != nil || result < 1 {
		// The error message doesn't fit but for OC10 API compatibility reasons we have to set this.
		return 0, fmt.Errorf("Cannot set %s of 0 or smaller!", name)
	}
	return int(result), nil
}

func (s *svc) Thumbnail(w http.ResponseWriter, r *http.Request) {
	thumbReq, err := s.parseThumbnailRequest(r)
	if err != nil {
		s.writeHTTPError(w, err)
		return
	}

	data, mimetype, err := s.thumbnail.GetThumbnail(r.Context(), thumbReq.File, thumbReq.Width, thumbReq.Height, thumbReq.OutputType)
	if err != nil {
		s.writeHTTPError(w, err)
		return
	}

	// send back the thumbnail in the body of the response
	buf := bytes.NewBuffer(data)
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", mimetype)
	_, err = io.Copy(w, buf)
	if err != nil {
		s.log.Error().Err(err).Msg("error writinh thumbnail into the response writer")
	}
}

func (s *svc) writeHTTPError(w http.ResponseWriter, err error) {
	s.log.Error().Err(err).Msg("thumbnails: got error")

	switch err.(type) {
	case errtypes.NotFound:
		w.WriteHeader(http.StatusNotFound)
	case errtypes.BadRequest:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}

	_, _ = w.Write([]byte(err.Error()))
}

func (s *svc) Handler() http.Handler {
	return s.router
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return nil
}
