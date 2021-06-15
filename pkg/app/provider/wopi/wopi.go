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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cs3org/reva/pkg/app"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("wopi", New)
}

type config struct {
	IOPSecret     string `mapstructure:"iop_secret" docs:";The IOP secret used to connect to the wopiserver."`
	WopiURL       string `mapstructure:"wopi_url" docs:";The wopiserver's URL."`
	WopiBridgeURL string `mapstructure:"wopi_bridge_url" docs:";The wopibridge's URL."`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

type wopiProvider struct {
	conf             *config
	client           *http.Client
	wopiBridgeClient *http.Client
}

func (p *wopiProvider) getWopiAppEndpoints(ctx context.Context) (map[string]interface{}, error) {
	// TODO this query will eventually be served by Reva.
	// For the time being it is a remnant of the CERNBox-specific WOPI server, which justifies the /cbox path in the URL.
	wopiurl, err := url.Parse(p.conf.WopiURL)
	if err != nil {
		return nil, err
	}
	wopiurl.Path = path.Join(wopiurl.Path, "/wopi/cbox/endpoints")
	appsReq, err := rhttp.NewRequest(ctx, "GET", wopiurl.String(), nil)
	if err != nil {
		return nil, err
	}
	appsRes, err := p.client.Do(appsReq)
	if err != nil {
		return nil, err
	}
	defer appsRes.Body.Close()
	if appsRes.StatusCode != http.StatusOK {
		return nil, errtypes.InternalError(fmt.Sprintf("Request to WOPI server returned %d", appsRes.StatusCode))
	}
	appsBody, err := ioutil.ReadAll(appsRes.Body)
	if err != nil {
		return nil, err
	}

	appsURLMap := make(map[string]interface{})
	err = json.Unmarshal(appsBody, &appsURLMap)
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	log.Info().Msg(fmt.Sprintf("Successfully retrieved %d WOPI app endpoints", len(appsURLMap)))
	return appsURLMap, nil
}

func (p *wopiProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.OpenInAppRequest_ViewMode, app, token string) (string, error) {
	log := appctx.GetLogger(ctx)

	wopiurl, err := url.Parse(p.conf.WopiURL)
	if err != nil {
		return "", err
	}
	wopiurl.Path = path.Join(wopiurl.Path, "/wopi/iop/open")
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
	q.Add("folderurl", "undefined")
	u, ok := user.ContextGetUser(ctx)
	if ok {
		q.Add("username", u.Username)
	}
	// else defaults to "Anonymous Guest"

	if p.conf.IOPSecret == "" {
		p.conf.IOPSecret = os.Getenv("REVA_APPPROVIDER_IOPSECRET")
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.conf.IOPSecret)
	httpReq.Header.Set("TokenHeader", token)

	httpReq.URL.RawQuery = q.Encode()

	openRes, err := p.client.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(err, "wopi: error performing open request to WOPI server")
	}
	defer openRes.Body.Close()

	if openRes.StatusCode != http.StatusOK {
		return "", errors.Wrap(err, "wopi: error performing open request to WOPI server, status: "+openRes.Status)
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(openRes.Body)
	if err != nil {
		return "", err
	}
	openResBody := buf.String()

	var viewModeStr string
	if viewMode == appprovider.OpenInAppRequest_VIEW_MODE_READ_WRITE {
		viewModeStr = "edit"
	} else {
		viewModeStr = "view"
	}

	var appProviderURL string
	if app == "" {
		// Default behavior: work out the application URL to be used for this file
		// TODO call this e.g. once a day or a week, and cache the content in a shared map protected by a multi-reader Lock
		appsURLMap, err := p.getWopiAppEndpoints(ctx)
		if err != nil {
			return "", errors.Wrap(err, "wopi: getWopiAppEndpoints failed")
		}
		viewOptions := appsURLMap[path.Ext(resource.GetPath())]
		viewOptionsMap, ok := viewOptions.(map[string]interface{})
		if !ok {
			return "", errtypes.InternalError("wopi: incorrect parsing of the App URLs map from the WOPI server")
		}

		appProviderURL = fmt.Sprintf("%v", viewOptionsMap[viewModeStr])
		if strings.Contains(appProviderURL, "?") {
			appProviderURL += "&"
		} else {
			appProviderURL += "?"
		}
		appProviderURL = fmt.Sprintf("%sWOPISrc=%s", appProviderURL, openResBody)
	} else {
		// User specified the application to use, generate the URL out of that
		// TODO map the given req.App to the URL via config. For now assume it's a URL!
		appProviderURL = fmt.Sprintf("%sWOPISrc=%s", app, openResBody)
	}

	// In case of applications served by the WOPI bridge, resolve the URL and go to the app
	// Note that URL matching is performed via string matching, not via IP resolution: may need to fix this
	if len(p.conf.WopiBridgeURL) > 0 && strings.Contains(appProviderURL, p.conf.WopiBridgeURL) {
		bridgeReq, err := rhttp.NewRequest(ctx, "GET", appProviderURL, nil)
		if err != nil {
			return "", err
		}
		bridgeRes, err := p.wopiBridgeClient.Do(bridgeReq)
		if err != nil {
			return "", err
		}
		defer bridgeRes.Body.Close()
		if bridgeRes.StatusCode != http.StatusFound {
			return "", errtypes.InternalError(fmt.Sprintf("Request to WOPI bridge returned %d", bridgeRes.StatusCode))
		}
		appProviderURL = bridgeRes.Header.Get("Location")
	}

	log.Info().Msg(fmt.Sprintf("wopi: returning app provider URL %s", appProviderURL))
	return appProviderURL, nil
}

// New returns an implementation to of the app.Provider interface that
// connects to an application in the backend.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	wopiBridgeClient := rhttp.GetHTTPClient(
		rhttp.Timeout(time.Duration(5 * int64(time.Second))),
	)
	wopiBridgeClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &wopiProvider{
		conf: c,
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(5 * time.Second),
		),
		wopiBridgeClient: wopiBridgeClient,
	}, nil
}
