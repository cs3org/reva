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
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
	"github.com/gdexlab/go-render/render"
	ua "github.com/mileusna/useragent"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type svc struct {
	config     *Config
	httpClient *http.Client
	gtwClient  gateway.GatewayAPIClient
	log        *zerolog.Logger
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix      string `mapstructure:"prefix"`
	GatewaySvc  string `mapstructure:"gatewaysvc"`
	Timeout     int64  `mapstructure:"timeout"`
	Insecure    bool   `mapstructure:"insecure"`
	MaxNumFiles int64  `mapstructure:"max_num_files"`
	MaxSize     int64  `mapstructure:"max_size"`
}

const (
	errMaxFileCount = errtypes.InternalError("reached max files count")
	errMaxSize      = errtypes.InternalError("reached max total files size")
)

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
		config:    c,
		gtwClient: gtw,
		httpClient: rhttp.GetHTTPClient(
			rhttp.Timeout(time.Duration(c.Timeout*int64(time.Second))),
			rhttp.Insecure(c.Insecure),
		),
		log: log,
	}, nil
}

func (c *Config) init() {
	if c.Prefix == "" {
		c.Prefix = "download_archive"
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

		dir := getDeepestCommonDir(files)
		if pathIn(files, dir) {
			dir = filepath.Dir(dir)
		}

		userAgent := ua.Parse(r.Header.Get("User-Agent"))

		archiveName := "download"
		if userAgent.OS == ua.Windows {
			archiveName += ".zip"
		} else {
			archiveName += ".tar"
		}

		s.log.Debug().Msg("Requested the following files/folders to archive: " + render.Render(files))

		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", archiveName))
		rw.Header().Set("Content-Transfer-Encoding", "binary")

		if userAgent.OS == ua.Windows {
			err = s.createZip(ctx, dir, files, rw)
		} else {
			err = s.createTar(ctx, dir, files, rw)
		}
		if err == errMaxFileCount || err == errMaxSize {
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

// create a new tar containing the files in the `files` list, which are in the directory `dir`
func (s *svc) createTar(ctx context.Context, dir string, files []string, dst io.Writer) error {
	w := tar.NewWriter(dst)

	var filesCount, sizeFiles int64

	for _, root := range files {

		err := walker.Walk(ctx, root, s.gtwClient, func(path string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				return err
			}

			filesCount++
			if filesCount > s.config.MaxNumFiles {
				return errMaxFileCount
			}
			sizeFiles += int64(info.Size)
			if sizeFiles > s.config.MaxSize {
				return errMaxSize
			}

			// TODO (gdelmont): remove duplicates if the resources requested overlaps
			fileName, err := filepath.Rel(dir, path)

			if err != nil {
				return err
			}

			header := tar.Header{
				Name:    fileName,
				ModTime: time.Unix(int64(info.Mtime.Seconds), 0),
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			if isDir {
				// the resource is a folder
				header.Mode = 0755
				header.Typeflag = tar.TypeDir
			} else {
				header.Mode = 0644
				header.Typeflag = tar.TypeReg
				header.Size = int64(info.Size)
			}

			err = w.WriteHeader(&header)
			if err != nil {
				return err
			}

			if !isDir {
				err = s.downloadFile(ctx, path, w)
				if err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}

	}
	return w.Close()
}

// create a new zip containing the files in the `files` list, which are in the directory `dir`
func (s *svc) createZip(ctx context.Context, dir string, files []string, dst io.Writer) error {
	w := zip.NewWriter(dst)

	var filesCount, sizeFiles int64

	for _, root := range files {

		err := walker.Walk(ctx, root, s.gtwClient, func(path string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				return err
			}

			filesCount++
			if filesCount > s.config.MaxNumFiles {
				return errMaxFileCount
			}
			sizeFiles += int64(info.Size)
			if sizeFiles > s.config.MaxSize {
				return errMaxSize
			}

			// TODO (gdelmont): remove duplicates if the resources requested overlaps
			fileName, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}

			if fileName == "" {
				return nil
			}

			header := zip.FileHeader{
				Name:     fileName,
				Modified: time.Unix(int64(info.Mtime.Seconds), 0),
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			if isDir {
				header.Name += "/"
			} else {
				header.UncompressedSize64 = info.Size
			}

			dst, err := w.CreateHeader(&header)
			if err != nil {
				return err
			}

			if !isDir {
				err = s.downloadFile(ctx, path, dst)
				if err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}

	}
	return w.Close()
}

func (s *svc) downloadFile(ctx context.Context, path string, dst io.Writer) error {
	downResp, err := s.gtwClient.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return err
	case downResp.Status.Code != rpc.Code_CODE_OK:
		return errtypes.InternalError(downResp.Status.Message)
	}

	var endpoint, token string
	for _, p := range downResp.Protocols {
		if p.Protocol == "simple" {
			endpoint, token = p.DownloadEndpoint, p.Token
		}
	}

	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := s.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		return errtypes.InternalError(httpRes.Status)
	}

	_, err = io.Copy(dst, httpRes.Body)
	return err
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

func pathIn(files []string, f string) bool {
	cleanedF := filepath.Clean(f)
	for _, file := range files {
		if filepath.Clean(file) == cleanedF {
			return true
		}
	}
	return false
}

func getDeepestCommonDir(files []string) string {

	if len(files) == 0 {
		return ""
	}

	// find the maximum common substring from left
	res := path.Clean(files[0]) + "/"

	for _, file := range files[1:] {
		file = path.Clean(file) + "/"

		if len(file) < len(res) {
			res, file = file, res
		}

		for i := 0; i < len(res); i++ {
			if res[i] != file[i] {
				res = res[:i]
			}
		}

	}

	// the common substring could be between two / - inside a file name
	for i := len(res) - 1; i >= 0; i-- {
		if res[i] == '/' {
			res = res[:i+1]
			break
		}
	}
	return filepath.Clean(res)
}
