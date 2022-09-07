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
	"path/filepath"
	"strconv"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	share "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/thumbnails/manager"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
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
	"google.golang.org/grpc/metadata"
)

func init() {
	global.Register("thumbnails", New)
}

const (
	// DefaultWidth is the default width when the HTTP request is missing the width
	DefaultWidth int = 32
	// DefaultHeight is the default height when the HTTP request is missing the height
	DefaultHeight int = 32
)

type config struct {
	GatewaySVC   string                            `mapstructure:"gateway_svc"`
	Quality      int                               `mapstructure:"quality"`
	Resolutions  []string                          `mapstructure:"quality"`
	Cache        string                            `mapstructure:"cache"`
	CacheDrivers map[string]map[string]interface{} `mapstructure:"cache_drivers"`
	OutputType   string                            `mapstructure:"output_type"`
	Prefix       string                            `mapstructure:"prefix"`
	Insecure     bool                              `mapstructure:"insecure"`
}

type svc struct {
	c         *config
	router    *chi.Mux
	log       *zerolog.Logger
	client    gateway.GatewayAPIClient
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

// New creates a new thumbnails HTTP service
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
		client:    gtw,
	}

	// thumbnails for normal files
	r.Group(func(r chi.Router) {
		r.Use(s.davUserContext)
		r.Get("/files/*", s.Thumbnail)
	})

	// thumbnails for public links
	r.Group(func(r chi.Router) {
		r.Use(s.davPublicContext)
		r.Get("/public-files/*", s.Thumbnail)
		r.Head("/public-files/*", s.Thumbnail)
	})

	return s, nil
}

func (s *svc) davUserContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		path := chi.URLParam(r, "*")
		path, _ = url.QueryUnescape(path)
		path = "/" + path

		res, err := s.statRes(ctx, &provider.Reference{
			Path: path,
		})
		if err != nil {
			s.writeHTTPError(w, err)
			return
		}

		ctx = ContextSetResource(ctx, res)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *svc) davPublicContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		req := chi.URLParam(r, "*")
		tknPath := strings.SplitN(req, "/", 2)
		var token, path string

		switch len(tknPath) {
		case 2:
			path = tknPath[1]
			fallthrough
		case 1:
			token = tknPath[0]
		default:
			s.writeHTTPError(w, errtypes.BadRequest("no token provided"))
		}

		// TODO: add support for public shares with password
		rsp, err := s.client.Authenticate(ctx, &gateway.AuthenticateRequest{
			Type:     "publicshares",
			ClientId: token,
			// We pass an empty password because we expect non pre-signed public links
			// to not be password protected
			ClientSecret: "password|",
		})
		if err != nil {
			s.writeHTTPError(w, err)
			return
		}

		path = filepath.Join("/public", token, path)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, rsp.Token)

		res, err := s.statPublicFile(ctx, token, path)
		if err != nil {
			s.writeHTTPError(w, err)
			return
		}

		ctx = ContextSetResource(ctx, res)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *svc) statPublicFile(ctx context.Context, token, path string) (*provider.ResourceInfo, error) {
	resp, err := s.client.GetPublicShare(ctx, &share.GetPublicShareRequest{
		Ref: &share.PublicShareReference{
			Spec: &share.PublicShareReference_Token{
				Token: token,
			},
		},
	})

	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(token)
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(fmt.Sprintf("error getting public share %s", token))
	}

	d := filepath.Dir(path)

	res, err := s.statRes(ctx, &provider.Reference{
		ResourceId: resp.Share.ResourceId,
		Path:       d,
	})
	if err != nil {
		return nil, err
	}

	if res.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		return res, err
	}

	return s.statRes(ctx, &provider.Reference{
		ResourceId: resp.Share.ResourceId,
		Path:       path,
	})
}

func (s *svc) statRes(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	resp, err := s.client.Stat(ctx, &provider.StatRequest{
		Ref: ref,
	})
	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(fmt.Sprintf("%+v", ref))
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(fmt.Sprintf("error statting resource %+v", ref))
	}

	return resp.Info, nil
}

type thumbnailRequest struct {
	File       string
	ETag       string
	Width      int
	Height     int
	OutputType manager.FileType
}

func (s *svc) parseThumbnailRequest(r *http.Request) (*thumbnailRequest, error) {
	ctx := r.Context()

	res := ContextMustGetResource(ctx)

	if res.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		return nil, errtypes.BadRequest("resource is not a file")
	}

	width, height, err := parseDimensions(r.URL.Query())
	if err != nil {
		return nil, errtypes.BadRequest(fmt.Sprintf("error parsing dimensions: %v", err))
	}

	t := getOutType(s.c.OutputType)

	return &thumbnailRequest{
		File:       res.Path,
		ETag:       res.Etag,
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
		return 0, fmt.Errorf("cannot set %s of 0 or smaller", name)
	}
	return int(result), nil
}

// Thumbnail generates a thumbnail of the resource in the request
func (s *svc) Thumbnail(w http.ResponseWriter, r *http.Request) {
	thumbReq, err := s.parseThumbnailRequest(r)
	if err != nil {
		s.writeHTTPError(w, err)
		return
	}

	data, mimetype, err := s.thumbnail.GetThumbnail(r.Context(), thumbReq.File, thumbReq.ETag, thumbReq.Width, thumbReq.Height, thumbReq.OutputType)
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
	return []string{"/public-files"}
}
