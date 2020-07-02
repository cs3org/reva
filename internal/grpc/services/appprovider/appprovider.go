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

package appprovider

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/demo"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("appprovider", New)
}

type service struct {
	provider app.Provider
	conf     *config
}

type config struct {
	Driver          string                 `mapstructure:"driver"`
	Demo            map[string]interface{} `mapstructure:"demo"`
	IopSecret       string                 `mapstructure:"iopsecret" docs:"The iopsecret used to connect to the wopiserver."`
	WopiURL         string                 `mapstructure:"wopiurl" docs:"The wopiserver's url."`
	UIURL           string                 `mapstructure:"uirul" docs:"URL to application (eg collabora) URL."`
	StorageEndpoint string                 `mapstructure:"storageendpoint" docs:"The storage endpoint used by the wopiserver 
	to look up the file or storage id, defaults to "default" by the wopiserver if empty."`
}

// New creates a new AppProviderService
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	provider, err := getProvider(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:     c,
		provider: provider,
	}

	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	providerpb.RegisterProviderAPIServer(ss, s)
}
func getProvider(c *config) (app.Provider, error) {
	switch c.Driver {
	case "demo":
		return demo.New(c.Demo)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}

func (s *service) OpenFileInAppProvider(ctx context.Context, req *providerpb.OpenFileInAppProviderRequest) (*providerpb.OpenFileInAppProviderResponse, error) {

	log := appctx.GetLogger(ctx)

	wopiurl := s.conf.WopiURL
	iopsecret := s.conf.IopSecret
	storageEndpoint := s.conf.StorageEndpoint
	folderURL := s.conf.UIURL + filepath.Dir(req.Ref.GetPath())

	httpClient := rhttp.GetHTTPClient(
		rhttp.Context(ctx),
		// TODO make insecure configurable
		rhttp.Insecure(true),
		// TODO make timeout configurable
		rhttp.Timeout(time.Duration(24*int64(time.Hour))),
	)
	httpReq, err := rhttp.NewRequest(ctx, "GET", wopiurl+"wopi/iop/open", nil)

	q := httpReq.URL.Query()
	q.Add("filename", req.Ref.GetPath())
	q.Add("endpoint", storageEndpoint)
	q.Add("viewmode", req.ViewMode.String())
	q.Add("folderurl", folderURL)

	httpReq.Header.Set("Authorization", "Bearer "+iopsecret)
	httpReq.Header.Set("TokenHeader", req.AccessToken)

	if err != nil {
		return nil, err
	}
	httpReq.URL.RawQuery = q.Encode()
	httpRes, err := httpClient.Do(httpReq)

	if err != nil {
		log.Error().Err(err).Msg("error performing http request")
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error performing http request"),
		}
		return res, nil
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		log.Error().Err(err).Msg("error performing http request")
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error performing http request, status code: "+strconv.Itoa(httpRes.StatusCode)),
		}
		return res, nil
	}

	if err != nil {
		err := errors.Wrap(err, "appprovidersvc: error calling GetIFrame")
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error getting app's iframe"),
		}
		return res, nil
	}

	resBody, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		err := errors.Wrap(err, "appprovidersvc: error reading reponse body")
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error reading reponse body"),
		}
		return res, nil
	}

	return &providerpb.OpenFileInAppProviderResponse{
		Status:         status.NewOK(ctx),
		AppProviderUrl: string(resBody),
	}, nil
}
