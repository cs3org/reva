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

package archiver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/archiver/manager"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/v3/pkg/storage/utils/walker"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/gdexlab/go-render/render"
	ua "github.com/mileusna/useragent"
)

type svc struct {
	config     *Config
	gtwClient  gateway.GatewayAPIClient
	walker     walker.Walker
	downloader downloader.Downloader

	allowedFolders []*regexp.Regexp
}

// Config holds the config options that need to be passed down to all ocdav handlers.
type Config struct {
	Prefix         string   `mapstructure:"prefix"`
	GatewaySvc     string   `mapstructure:"gatewaysvc"                                              validate:"required"`
	Timeout        int64    `mapstructure:"timeout"`
	Insecure       bool     `docs:"false;Whether to skip certificate checks when sending requests." mapstructure:"insecure"`
	Name           string   `mapstructure:"name"`
	MaxNumFiles    int64    `mapstructure:"max_num_files"                                           validate:"required,gt=0"`
	MaxSize        int64    `mapstructure:"max_size"                                                validate:"required,gt=0"`
	AllowedFolders []string `mapstructure:"allowed_folders"`
}

func init() {
	global.Register("archiver", New)
}

// New creates a new archiver service.
func New(ctx context.Context, conf map[string]interface{}) (global.Service, error) {
	var c Config
	if err := cfg.Decode(conf, &c); err != nil {
		return nil, err
	}

	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return nil, err
	}

	// compile all the regex for filtering folders
	allowedFolderRegex := make([]*regexp.Regexp, 0, len(c.AllowedFolders))
	for _, s := range c.AllowedFolders {
		regex, err := regexp.Compile(s)
		if err != nil {
			return nil, err
		}
		allowedFolderRegex = append(allowedFolderRegex, regex)
	}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure}}
	hc := httpclient.New(httpclient.RoundTripper(tr), httpclient.Timeout(time.Duration(c.Timeout*int64(time.Second))))

	return &svc{
		config:         &c,
		gtwClient:      gtw,
		downloader:     downloader.NewDownloader(gtw, hc),
		walker:         walker.NewWalker(gtw),
		allowedFolders: allowedFolderRegex,
	}, nil
}

func (c *Config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "archiver"
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

	f := make([]string, 0, len(files)+len(ids))

	for _, id := range ids {
		// id is base64 encoded and after decoding has the form <storage_id>!<resource_id>

		ref, ok := spaces.ParseResourceID(id)
		if !ok {
			// If this fails, client might be non-spaces
			var err error
			ref, err = spaces.ResourceIdFromString(id)
			if err != nil {
				return nil, errors.New("could not unwrap given file id")
			}
		}

		resp, err := s.gtwClient.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				ResourceId: ref,
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

	f = append(f, files...)

	// check if all the folders are allowed to be archived
	err := s.allAllowed(f)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// return true if path match with at least with one allowed folder regex.
func (s *svc) isPathAllowed(path string) bool {
	for _, reg := range s.allowedFolders {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

// return nil if all the paths in the slide match with at least one allowed folder regex.
func (s *svc) allAllowed(paths []string) error {
	if len(s.allowedFolders) == 0 {
		return nil
	}

	for _, f := range paths {
		if !s.isPathAllowed(f) {
			return errtypes.BadRequest(fmt.Sprintf("resource at %s not allowed to be archived", f))
		}
	}
	return nil
}

func (s *svc) writeHTTPError(ctx context.Context, w http.ResponseWriter, err error) {
	log := appctx.GetLogger(ctx)
	log.Error().Msg(err.Error())

	switch err.(type) {
	case errtypes.NotFound:
		w.WriteHeader(http.StatusNotFound)
	case manager.ErrMaxSize, manager.ErrMaxFileCount:
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	case errtypes.BadRequest:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}

	_, _ = w.Write([]byte(err.Error()))
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// get the paths and/or the resources id from the query
		ctx := r.Context()
		log := appctx.GetLogger(ctx)
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
			s.writeHTTPError(ctx, rw, err)
			return
		}

		arch, err := manager.NewArchiver(files, s.walker, s.downloader, manager.Config{
			MaxNumFiles: s.config.MaxNumFiles,
			MaxSize:     s.config.MaxSize,
		})
		if err != nil {
			s.writeHTTPError(ctx, rw, err)
			return
		}

		archType := v.Get("arch_type") // optional, either "tar" or "zip"
		if archType == "" || archType != "tar" && archType != "zip" {
			// in case of missing or bogus arch_type, detect it via user-agent
			userAgent := ua.Parse(r.Header.Get("User-Agent"))
			if userAgent.OS == ua.Windows {
				archType = "zip"
			} else {
				archType = "tar"
			}
		}

		var archName string
		if len(files) == 1 {
			archName = strings.TrimSuffix(filepath.Base(files[0]), filepath.Ext(files[0])) + "." + archType
		} else {
			// TODO(lopresti) we may want to generate a meaningful name out of the list
			archName = s.config.Name + "." + archType
		}

		log.Debug().Msg("Requested the following files/folders to archive: " + render.Render(files))

		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", archName))
		rw.Header().Set("Content-Transfer-Encoding", "binary")

		// create the archive
		if archType == "zip" {
			err = arch.CreateZip(ctx, rw)
		} else {
			err = arch.CreateTar(ctx, rw)
		}

		if err != nil {
			s.writeHTTPError(ctx, rw, err)
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
