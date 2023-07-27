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
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

type overleafProvider struct {
	conf *config
}

func (p *overleafProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.ViewMode, token string, opaqueMap map[string]*typespb.OpaqueEntry, language string) (*appprovider.OpenInAppURL, error) {
	log := appctx.GetLogger(ctx)

	// we need to generate a more restricted token
	restrictedToken := token

	// getting file/folder name so as not to expose authentication token in project name
	name := resource.Path[strings.LastIndex(resource.Path, "/")+1:]
	name = strings.Split(name, ".")[0] // removing extension resource has one

	url := fmt.Sprintf("%s/docs?snip_uri=%s/archiver?id=%s!%s&access_token=%s&snip_name=%s", p.conf.AppURL, p.conf.FolderBaseURL, resource.Id.StorageId, resource.Id.OpaqueId, restrictedToken, name)

	log.Debug().Str("url", url).Msg("Returning URL for creating a project")
	return &appprovider.OpenInAppURL{
		AppUrl: url,
		Method: http.MethodGet,
	}, nil
}

func (p *overleafProvider) GetAppProviderInfo(ctx context.Context) (*appregistry.ProviderInfo, error) {
	return &appregistry.ProviderInfo{
		Name:        "Overleaf",
		MimeTypes:   p.conf.MimeTypes,
		Icon:        p.conf.AppIconURI,
		Description: "Export to",
	}, nil
}

func init() {
	registry.Register("overleaf", New)
}

type config struct {
	MimeTypes           []string `mapstructure:"mime_types" docs:"nil;Inherited from the appprovider."`
	AppName             string   `mapstructure:"app_name" docs:";The App user-friendly name."`
	AppIconURI          string   `mapstructure:"app_icon_uri" docs:";A URI to a static asset which represents the app icon."`
	FolderBaseURL       string   `mapstructure:"folder_base_url" docs:"; Public internet facing URL used to serve the files to Overleaf."`
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
func New(ctx context.Context, m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &overleafProvider{conf: c}, nil
}
