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

package archiver

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/internal/http/services/archiver/manager"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
	"github.com/gdexlab/go-render/render"
	ua "github.com/mileusna/useragent"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type svc struct {
	config     *Config
	gtwClient  gateway.GatewayAPIClient
	log        *zerolog.Logger
	walker     walker.Walker
	downloader downloader.Downloader
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix      string `mapstructure:"prefix"`
	GatewaySvc  string `mapstructure:"gatewaysvc"`
	Timeout     int64  `mapstructure:"timeout"`
	Insecure    bool   `mapstructure:"insecure"`
	Name        string `mapstructure:"name"`
	MaxNumFiles int64  `mapstructure:"max_num_files"`
	MaxSize     int64  `mapstructure:"max_size"`
}

func init() {
	global.Register("archiver", New)
}

// New creates a new archiver service
func New(conf map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c := &Config{}
	err := mapstructure.Decode(conf, c)
	if err != nil {
		return nil, err
	}

	c.init()

	gtw, err := pool.GetGatewayServiceClient(c.GatewaySvc)
	if err != nil {
		return nil, err
	}

	return &svc{
		config:     c,
		gtwClient:  gtw,
		downloader: downloader.NewDownloader(gtw, rhttp.Insecure(c.Insecure), rhttp.Timeout(time.Duration(c.Timeout*int64(time.Second)))),
		walker:     walker.NewWalker(gtw),
		log:        log,
	}, nil
}

func (c *Config) init() {
	if c.Prefix == "" {
		c.Prefix = "download_archive"
	}

	if c.Name == "" {
		c.Name = "download"
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

func (s *svc) getFiles(ctx context.Context, files, ids []string) ([]string, error) {
	if len(files) == 0 && len(ids) == 0 {
		return nil, errtypes.BadRequest("file and id lists are both empty")
	}

	f := []string{}

	for _, id := range ids {
		// id is base64 encoded and after decoding has the form <storage_id>:<resource_id>

		storageID, opaqueID, err := decodeResourceID(id)
		if err != nil {
			return nil, err
		}

		resp, err := s.gtwClient.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: storageID,
					OpaqueId:  opaqueID,
				},
			},
		})

		switch {
		case err != nil:
			return nil, err
		case resp.Status.Code == rpc.Code_CODE_NOT_FOUND:
			return nil, errtypes.NotFound(id)
		case resp.Status.Code != rpc.Code_CODE_OK:
			return nil, errtypes.InternalError(fmt.Sprintf("error getting stats from %s", id))
		}

		f = append(f, resp.Info.Path)

	}

	return append(f, files...), nil
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// get the paths and/or the resources id from the query
		ctx := r.Context()
		v := r.URL.Query()

		paths, ok := v["path"]
		if !ok {
			paths = []string{}
		}
		ids, ok := v["id"]
		if !ok {
			ids = []string{}
		}

		files, err := s.getFiles(ctx, paths, ids)
		if err != nil {
			s.log.Error().Msg(err.Error())
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		arch, err := manager.NewArchiver(files, s.walker, s.downloader, manager.Config{
			MaxNumFiles: s.config.MaxNumFiles,
			MaxSize:     s.config.MaxSize,
		})
		if err != nil {
			s.log.Error().Msg(err.Error())
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		userAgent := ua.Parse(r.Header.Get("User-Agent"))

		archName := s.config.Name
		if userAgent.OS == ua.Windows {
			archName += ".zip"
		} else {
			archName += ".tar"
		}

		s.log.Debug().Msg("Requested the following files/folders to archive: " + render.Render(files))

		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", archName))
		rw.Header().Set("Content-Transfer-Encoding", "binary")

		// create the archive
		if userAgent.OS == ua.Windows {
			err = arch.CreateZip(ctx, rw)
		} else {
			err = arch.CreateTar(ctx, rw)
		}

		if err == manager.ErrMaxFileCount || err == manager.ErrMaxSize {
			s.log.Error().Msg(err.Error())
			rw.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			s.log.Error().Msg(err.Error())
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

	})
}

func (s *svc) Prefix() string {
	return s.config.Prefix
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return nil
}

func decodeResourceID(encodedID string) (string, string, error) {
	decodedID, err := base64.URLEncoding.DecodeString(encodedID)
	if err != nil {
		return "", "", errors.Wrap(err, "resource ID does not follow the required format")
	}

	parts := strings.Split(string(decodedID), ":")
	if len(parts) != 2 {
		return "", "", errtypes.BadRequest("resource ID does not follow the required format")
	}
	if !utf8.ValidString(parts[0]) || !utf8.ValidString(parts[1]) {
		return "", "", errtypes.BadRequest("resourceID contains illegal characters")
	}
	return parts[0], parts[1], nil
}
