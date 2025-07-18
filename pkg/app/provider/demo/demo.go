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

package demo

import (
	"context"
	"fmt"
	"net/http"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/app"
	"github.com/cs3org/reva/v3/pkg/app/provider/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	registry.Register("demo", New)
}

type demoProvider struct {
	iframeUIProvider string
}

func (p *demoProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.ViewMode, token string, opaqueMap map[string]*typespb.OpaqueEntry, language string) (*appprovider.OpenInAppURL, error) {
	url := fmt.Sprintf("<iframe src=%s/open/%s?view-mode=%s&access-token=%s&lang=%s />", p.iframeUIProvider, resource.Id.StorageId+":"+resource.Id.OpaqueId, viewMode.String(), token, language)
	return &appprovider.OpenInAppURL{
		AppUrl: url,
		Method: http.MethodGet,
		Target: appprovider.Target_TARGET_IFRAME, // alternatively, appprovider.Target_TARGET_BLANK
	}, nil
}

func (p *demoProvider) GetAppProviderInfo(ctx context.Context) (*appregistry.ProviderInfo, error) {
	return &appregistry.ProviderInfo{
		Name:        "demo-app",
		Description: "A dummy app provider",
		MimeTypes:   []string{},
		Action:      "Demo open",
	}, nil
}

type config struct {
	IFrameUIProvider string `mapstructure:"iframe_ui_provider"`
}

// New returns an implementation to of the app.Provider interface that
// connects to an application in the backend.
func New(ctx context.Context, m map[string]interface{}) (app.Provider, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &demoProvider{iframeUIProvider: c.IFrameUIProvider}, nil
}
