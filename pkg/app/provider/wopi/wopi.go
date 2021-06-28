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

package demo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/cs3org/reva/pkg/app"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("wopi", New)
}

type config struct {
	IOPSecret    string `mapstructure:"iop_secret" docs:";The IOP secret used to connect to the wopiserver."`
	WopiURL      string `mapstructure:"wopi_url" docs:";The wopiserver's URL."`
	MSOOURL      string `mapstructure:"msoo_url" docs:";The MS Office Online URL."`
	CodeURL      string `mapstructure:"code_url" docs:";The Collabora Online URL."`
	CodiMDURL    string `mapstructure:"codimd_url" docs:";The CodiMD URL."`
	CodiMDIntURL string `mapstructure:"codimd_int_url" docs:";The CodiMD internal URL."`
	CodiMDApiKey string `mapstructure:"codimd_url" docs:";The CodiMD URL."`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

type wopiProvider struct {
	conf       *config
	wopiClient *http.Client
}

func (p *wopiProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.OpenInAppRequest_ViewMode, app, token string) (string, error) {
	log := appctx.GetLogger(ctx)

	wopiurl, err := url.Parse(p.conf.WopiURL)
	if err != nil {
		return "", err
	}
	wopiurl.Path = path.Join(wopiurl.Path, "/wopi/iop/openinapp")
	httpReq, err := rhttp.NewRequest(ctx, "GET", wopiurl.String(), nil)
	if err != nil {
		return "", err
	}

	q := httpReq.URL.Query()
	q.Add("fileid", resource.GetId().OpaqueId)
	q.Add("endpoint", resource.GetId().StorageId)
	q.Add("viewmode", viewMode.String())
	// TODO the folder URL should be resolved as e.g. `'https://cernbox.cern.ch/index.php/apps/files/?dir=' + filepath.Dir(req.Ref.GetPath())`
	// or should be deprecated/removed altogether, needs discussion and decision.
	// q.Add("folderurl", "...")
	u, ok := user.ContextGetUser(ctx)
	if ok {
		q.Add("username", u.Username)
	}
	// else defaults to "Anonymous Guest"
	if app == "" {
		// Default behavior: look for the default app for this file's mimetype
		// XXX TODO
		app = "Collabora Online"
	}
	q.Add("appname", app)
	if app == "CodiMD" {
		// This is served by the WOPI bridge extensions
		q.Add("appediturl", p.conf.CodiMDURL)
		if p.conf.CodiMDIntURL != "" {
			q.Add("appinturl", p.conf.CodiMDIntURL)
		}
		httpReq.Header.Set("ApiKey", p.conf.CodiMDApiKey)
	} else {
		// TODO get AppRegistry
		//q.Add("appediturl", AppRegistry.get(app).getEditUrl())
		//q.Add("appviewurl", AppRegistry.get(app).getViewUrl())
	}

	if p.conf.IOPSecret == "" {
		p.conf.IOPSecret = os.Getenv("REVA_APPPROVIDER_IOPSECRET")
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.conf.IOPSecret)
	httpReq.Header.Set("TokenHeader", token)

	httpReq.URL.RawQuery = q.Encode()
	openRes, err := p.wopiClient.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(err, "wopi: error performing open request to WOPI server")
	}
	defer openRes.Body.Close()

	if openRes.StatusCode != http.StatusFound {
		return "", errors.Wrap(err, "wopi: error performing open request to WOPI server, status: "+openRes.Status)
	}
	appURL := openRes.Header.Get("Location")

	log.Info().Msg(fmt.Sprintf("wopi: returning app URL %s", appURL))
	return appURL, nil
}

// New returns an implementation of the app.Provider interface that
// connects to an application in the backend.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	wopiClient := rhttp.GetHTTPClient(
		rhttp.Timeout(time.Duration(5 * int64(time.Second))),
	)
	wopiClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &wopiProvider{
		conf:       c,
		wopiClient: wopiClient,
	}, nil
}
