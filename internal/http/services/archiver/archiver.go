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
	"context"
	"io"
	"net/http"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/storage/utils/walk"
	"github.com/rs/zerolog"
)

type svc struct {
	config     *Config
	httpClient *http.Client
	gtwClient  gateway.GatewayAPIClient
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix     string `mapstructure:"prefix"`
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

func init() {
	global.Register("archiver", New)
}

func New(conf map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	return nil, nil
}

func (s *svc) Handler() http.Handler {
	return nil
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

func (s *svc) createTar(ctx context.Context, files []string, dst io.Writer) error {
	w := tar.NewWriter(dst)

	for _, root := range files {

		err := walk.Walk(ctx, root, s.gtwClient, func(path string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				return err
			}

			tarHeader := tar.Header{
				Name:    path,
				ModTime: time.Unix(int64(info.Mtime.Seconds), 0),
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			if isDir {
				// the resource is a folder
				tarHeader.Mode = 0755
				tarHeader.Typeflag = tar.TypeDir
			} else {
				tarHeader.Mode = 0644
				tarHeader.Typeflag = tar.TypeReg
				tarHeader.Size = int64(info.Size)
			}

			err = w.WriteHeader(&tarHeader)
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
	return nil
}

func (s *svc) downloadFile(ctx context.Context, path string, dst io.Writer) error {
	downReq, err := s.gtwClient.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return err
	case downReq.Status.Code != rpc.Code_CODE_OK:
		return errtypes.InternalError(downReq.Status.Message)
	}

	var endpoint, token string
	for _, p := range downReq.Protocols {
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
