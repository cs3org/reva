// Copyright 2018-2023 CERN
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

package overleaf

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type overleafProvider struct {
	conf *config
}

func (p *overleafProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.ViewMode, token string, opaqueMap map[string]*typespb.OpaqueEntry, language string) (*appprovider.OpenInAppURL, error) {
	log := appctx.GetLogger(ctx)

	// client used to set and get arbitrary metadata to keep track whether project has already been exported
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(sharedconf.GetGatewaySVC("")))
	if err != nil {
		return nil, errors.Wrap(err, "overleaf: error fetching gateway service client.")
	}

	if _, ok := opaqueMap["override"]; !ok {
		// Check if resource has already been exported to Overleaf

		statRes, err := client.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				ResourceId: resource.Id,
			},
		})
		if err != nil {
			return nil, errors.Wrap(err, "overleaf: error statting file.")
		}

		creationTime, alreadySet := statRes.Info.GetArbitraryMetadata().Metadata["reva.overleaf.time"]

		if alreadySet {
			return nil, errtypes.AlreadyExists("Project was already exported on:" + creationTime)
		}
	}

	// TODO: generate and use a more restricted token
	restrictedToken := token

	// Setting up archiver request
	archHttpReq, err := rhttp.NewRequest(ctx, http.MethodGet, p.conf.ArchiverURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "overleaf: error setting up http request.")
	}

	archQuery := archHttpReq.URL.Query()
	archQuery.Add("id", resource.Id.StorageId+"!"+resource.Id.OpaqueId)
	archQuery.Add("access_token", restrictedToken)
	archQuery.Add("arch_type", "zip")

	archHttpReq.URL.RawQuery = archQuery.Encode()
	log.Debug().Str("Archiver url", archHttpReq.URL.String()).Msg("URL for downloading zipped resource from archiver")

	// Setting up Overleaf request
	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, p.conf.AppURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "overleaf: error setting up http request")
	}

	q := httpReq.URL.Query()

	// snip_uri is link to archiver request
	q.Add("snip_uri", archHttpReq.URL.String())

	// getting file/folder name so as not to expose authentication token in project name
	name := strings.TrimSuffix(filepath.Base(resource.Path), filepath.Ext(resource.Path))
	q.Add("snip_name", name)

	httpReq.URL.RawQuery = q.Encode()
	url := httpReq.URL.String()

	req := &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{
			ResourceId: resource.Id,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"reva.overleaf.time": strconv.Itoa(int(time.Now().Unix())),
			},
		},
	}

	res, err := client.SetArbitraryMetadata(ctx, req)

	if err != nil {
		return nil, errors.Wrap(err, "overleaf: error setting arbitrary metadata")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("error: code=%+v msg=%q support_trace=%q", res.Status.Code, res.Status.Message, res.Status.Trace)
	}

	return &appprovider.OpenInAppURL{
		AppUrl: url,
		Method: http.MethodGet,
		Target: appprovider.Target_TARGET_BLANK,
	}, nil
}

func (p *overleafProvider) GetAppProviderInfo(ctx context.Context) (*appregistry.ProviderInfo, error) {
	return &appregistry.ProviderInfo{
		Name:      "Overleaf",
		MimeTypes: p.conf.MimeTypes,
		Icon:      p.conf.AppIconURI,
		Action:    "Export to",
	}, nil
}

func init() {
	registry.Register("overleaf", New)
}

type config struct {
	MimeTypes           []string `mapstructure:"mime_types" docs:"nil;Inherited from the appprovider."`
	AppName             string   `mapstructure:"app_name" docs:";The App user-friendly name."`
	AppIconURI          string   `mapstructure:"app_icon_uri" docs:";A URI to a static asset which represents the app icon."`
	ArchiverURL         string   `mapstructure:"archiver_url" docs:"; Internet-facing URL of the archiver service, used to serve the files to Overleaf."`
	AppURL              string   `mapstructure:"app_url" docs:";The App URL."`
	AppIntURL           string   `mapstructure:"app_int_url" docs:";The internal app URL in case of dockerized deployments. Defaults to AppURL"`
	InsecureConnections bool     `mapstructure:"insecure_connections"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the app.Provider interface that
// connects to an application in the backend.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &overleafProvider{conf: c}, nil
}
