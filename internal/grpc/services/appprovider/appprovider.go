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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/demo"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
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
	Driver    string                 `mapstructure:"driver"`
	Demo      map[string]interface{} `mapstructure:"demo"`
	IopSecret string                 `mapstructure:"iopsecret" docs:";The iopsecret used to connect to the wopiserver."`
	WopiURL   string                 `mapstructure:"wopiurl" docs:";The wopiserver's URL."`
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

	httpClient := rhttp.GetHTTPClient(
		rhttp.Context(ctx),
		// these calls are expected to take a very short time, 5s (though hardcoded) ought to be far enough
		rhttp.Timeout(time.Duration(5*int64(time.Second))),
	)

	// TODO this query will eventually be served by Reva. For the time being it is a remnant of the CERNBox-specific WOPI server,
	// which justifies the /cbox path in the URL. Also, it could be executed every ~1 day or week, not at each request.
	wopiurl, err := url.Parse(s.conf.WopiURL)
	if err != nil {
		return nil, err
	}
	wopiurl.Path = path.Join(wopiurl.Path, "/wopi/cbox/endpoints")
	appsReq, err := rhttp.NewRequest(ctx, "GET", wopiurl.String(), nil)
	if err != nil {
		return nil, err
	}
	appsRes, err := httpClient.Do(appsReq)
	if err != nil {
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error performing http request"),
		}
		return res, nil
	}
	defer appsRes.Body.Close()
	if appsRes.StatusCode != http.StatusOK {
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error performing http request, status code: "+strconv.Itoa(appsRes.StatusCode)),
		}
		return res, nil
	}

	appsBody, err := ioutil.ReadAll(appsRes.Body)
	if err != nil {
		return nil, err
	}

	wopiurl, err = url.Parse(s.conf.WopiURL)
	if err != nil {
		return nil, err
	}
	wopiurl.Path = path.Join(wopiurl.Path, "/wopi/iop/open")
	httpReq, err := rhttp.NewRequest(ctx, "GET", wopiurl.String(), nil)
	if err != nil {
		return nil, err
	}

	q := httpReq.URL.Query()
	q.Add("fileid", req.ResourceInfo.GetId().OpaqueId)
	q.Add("endpoint", req.ResourceInfo.GetId().StorageId)
	q.Add("viewmode", req.ViewMode.String())
	// TODO the folder URL should be resolved as e.g. `'https://cernbox.cern.ch/index.php/apps/files/?dir=' + filepath.Dir(req.Ref.GetPath())`
	// or should be deprecated/removed altogether, needs discussion and decision.
	q.Add("folderurl", "undefined")
	u, ok := user.ContextGetUser(ctx)
	if ok {
		q.Add("username", u.Username)
	}
	// else defaults to "Anonymous Guest"
	httpReq.Header.Set("Authorization", "Bearer "+s.conf.IopSecret)
	httpReq.Header.Set("TokenHeader", req.AccessToken)

	httpReq.URL.RawQuery = q.Encode()

	openRes, err := httpClient.Do(httpReq)

	if err != nil {
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "appprovider: error performing open request to WOPI"),
		}
		return res, nil
	}
	defer openRes.Body.Close()

	if openRes.StatusCode != http.StatusOK {
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInvalid(ctx, "appprovider: error performing open request to WOPI, status code: "+strconv.Itoa(openRes.StatusCode)),
		}
		return res, nil
	}

	buf := new(bytes.Buffer)
	_, err1 := buf.ReadFrom(openRes.Body)
	if err1 != nil {
		return nil, err1
	}

	openResBody := buf.String()

	// TODO follow up from TODO above
	appsBodyMap := make(map[string]interface{})
	err2 := json.Unmarshal(appsBody, &appsBodyMap)
	if err2 != nil {
		return nil, err2
	}

	fileExtension := path.Ext(req.ResourceInfo.GetPath())

	viewOptions := appsBodyMap[fileExtension]

	viewOptionsMap, ok := viewOptions.(map[string]interface{})
	if !ok {
		res := &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInvalid(ctx, "Incorrect parsing of the App URLs map from the WOPI server"),
		}
		return res, nil
	}

	var viewmode string
	if req.ViewMode == providerpb.OpenFileInAppProviderRequest_VIEW_MODE_READ_WRITE {
		viewmode = "edit"
	} else {
		viewmode = "view"
	}

	appProviderURL := fmt.Sprintf("%v", viewOptionsMap[viewmode])
	if strings.Contains(appProviderURL, "?") {
		appProviderURL += "&"
	} else {
		appProviderURL += "?"
	}
	appProviderURL = fmt.Sprintf("%sWOPISrc=%s", appProviderURL, openResBody)
	log.Info().Msg(fmt.Sprintf("Returning app provider URL %s", appProviderURL))

	return &providerpb.OpenFileInAppProviderResponse{
		Status:         status.NewOK(ctx),
		AppProviderUrl: appProviderURL,
	}, nil
}
