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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/beevik/etree"
	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("wopi", New)
}

type config struct {
	IOPSecret string `mapstructure:"iop_secret" docs:";The IOP secret used to connect to the wopiserver."`
	WopiURL   string `mapstructure:"wopi_url" docs:";The wopiserver's URL."`
	AppName   string `mapstructure:"app_name" docs:";The App user-friendly name."`
	AppURL    string `mapstructure:"app_url" docs:";The App URL."`
	AppIntURL string `mapstructure:"app_int_url" docs:";The App internal URL in case of dockerized deployments. Defaults to AppURL"`
	AppAPIKey string `mapstructure:"app_api_key" docs:";The API key used by the App, if applicable."`
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
	appURLs    map[string]map[string]string // map[viewMode]map[extension]appURL
}

// New returns an implementation of the app.Provider interface that
// connects to an application in the backend.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	if c.AppIntURL == "" {
		c.AppIntURL = c.AppURL
	}

	appURLs, err := getAppURLs(c)
	if err != nil {
		return nil, err
	}

	err = registerApp(c, appURLs)
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
		appURLs:    appURLs,
	}, nil
}

func (p *wopiProvider) GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.OpenInAppRequest_ViewMode, app string, token string) (string, error) {
	log := appctx.GetLogger(ctx)

	if app != "" && app != p.conf.AppName {
		// Sanity check
		return "", errors.New("AppProvider for " + p.conf.AppName + " cannot open in " + app)
	}

	ext := path.Ext(resource.Path)
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
	if ok { // else defaults to "Anonymous Guest"
		q.Add("username", u.Username)
	}

	q.Add("appname", app)
	q.Add("appurl", p.appURLs["edit"][ext])

	if p.conf.AppIntURL != "" {
		q.Add("appinturl", p.conf.AppIntURL)
	}
	if viewExts, ok := p.appURLs["view"]; ok {
		if url, ok := viewExts[ext]; ok {
			q.Add("appviewurl", url)
		}
	}
	if p.conf.AppAPIKey != "" {
		httpReq.Header.Set("ApiKey", p.conf.AppAPIKey)
	}
	if p.conf.IOPSecret == "" {
		p.conf.IOPSecret = os.Getenv("REVA_APPPROVIDER_IOPSECRET")
	}
	httpReq.URL.RawQuery = q.Encode()

	httpReq.Header.Set("Authorization", "Bearer "+p.conf.IOPSecret)
	httpReq.Header.Set("TokenHeader", token)

	openRes, err := p.wopiClient.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(err, "wopi: error performing open request to WOPI server")
	}
	defer openRes.Body.Close()

	if openRes.StatusCode != http.StatusFound {
		return "", errors.Wrap(err, "wopi: unexpected status from WOPI server: "+openRes.Status)
	}
	appFullURL := openRes.Header.Get("Location")

	log.Info().Msg(fmt.Sprintf("wopi: returning app URL %s", appFullURL))
	return appFullURL, nil
}

func getAppURLs(c *config) (map[string]map[string]string, error) {
	// Initialize WOPI URLs by discovery
	httpcl := rhttp.GetHTTPClient(
		rhttp.Timeout(time.Duration(5 * int64(time.Second))),
	)

	appurl, err := url.Parse(c.AppIntURL)
	if err != nil {
		return nil, err
	}
	appurl.Path = path.Join(appurl.Path, "/hosting/discovery")

	discReq, err := http.NewRequest("GET", appurl.String(), nil)
	if err != nil {
		return nil, err
	}
	discRes, err := httpcl.Do(discReq)
	if err != nil {
		return nil, err
	}
	defer discRes.Body.Close()

	var appURLs map[string]map[string]string

	if discRes.StatusCode == http.StatusNotFound {
		// this may be a bridge-supported app
		discReq, err = http.NewRequest("GET", c.AppIntURL, nil)
		if err != nil {
			return nil, err
		}
		discRes, err = httpcl.Do(discReq)
		if err != nil {
			return nil, err
		}
		defer discRes.Body.Close()

		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(discRes.Body)
		if err != nil {
			return nil, err
		}

		// scrape app's home page to find the appname
		if !strings.Contains(buf.String(), c.AppName) {
			// || (c.AppName != "CodiMD" && c.AppName != "Etherpad") {
			return nil, errors.New("Application server at " + c.AppURL + " does not match this AppProvider for " + c.AppName)
		}

		// register the supported mimetypes in the AppRegistry: this is hardcoded for the time being
		if c.AppName == "CodiMD" {
			appURLs = getCodimdExtensions(c.AppURL)
		} else if c.AppName == "Etherpad" {
			appURLs = getEtherpadExtensions(c.AppURL)
		}
	} else if discRes.StatusCode == http.StatusOK {
		var netZoneName string
		if c.AppName == "Collabora" {
			netZoneName = "external-http"

		} else if c.AppName == "Office Online" {
			netZoneName = "external-https"
		}
		appURLs, err = parseWopiDiscovery(discRes.Body, netZoneName)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing wopi discovery response")
		}
	}
	return appURLs, nil
}

func registerApp(c *config, appURLs map[string]map[string]string) error {
	// Initially we store the mime types in a map to avoid duplicates
	mimeTypesMap := make(map[string]bool)
	for _, extensions := range appURLs {
		for ext := range extensions {
			m := mime.Detect(false, ext)
			mimeTypesMap[m] = true
		}
	}
	// TODO register these mimetypes in the AppRegistry
	mimeTypes := make([]string, 0, len(mimeTypesMap))
	for m := range mimeTypesMap {
		mimeTypes = append(mimeTypes, m)
	}
	return nil
}

func parseWopiDiscovery(body io.Reader, netZoneName string) (map[string]map[string]string, error) {
	appURLs := make(map[string]map[string]string)

	doc := etree.NewDocument()
	if _, err := doc.ReadFrom(body); err != nil {
		return nil, err
	}
	root := doc.SelectElement("wopi-discovery")

	for _, netZone := range root.SelectElements("net-zone") {
		nameAttr := netZone.SelectAttr("name")

		if nameAttr.Value == netZoneName {
			for _, app := range netZone.SelectElements("app") {
				for _, action := range app.SelectElements("action") {
					access := action.SelectAttrValue("name", "")
					if access == "view" || access == "edit" {
						ext := action.SelectAttrValue("ext", "")
						url := action.SelectAttrValue("urlsrc", "")

						if ext == "" || url == "" {
							continue
						}

						if _, ok := appURLs[access]; !ok {
							appURLs[access] = make(map[string]string)
						}
						appURLs[access][ext] = url
					}
				}
			}
		}
	}
	return appURLs, nil
}

func getCodimdExtensions(appURL string) map[string]map[string]string {
	appURLs := make(map[string]map[string]string)
	appURLs["edit"] = map[string]string{
		"txt": appURL,
		"md":  appURL,
		"zmd": appURL,
	}
	return appURLs
}

func getEtherpadExtensions(appURL string) map[string]map[string]string {
	appURLs := make(map[string]map[string]string)
	appURLs["edit"] = map[string]string{
		"etherpad": appURL,
	}
	return appURLs
}
