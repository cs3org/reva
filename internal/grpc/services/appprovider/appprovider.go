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
	"net/http"
	"net/url"
	"strings"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/demo"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("appprovider", New)
}

type service struct {
	provider app.Provider
}

type config struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
}

// New creates a new StorageRegistryService
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

func (s *service) Open(ctx context.Context, req *providerpb.OpenRequest) (*providerpb.OpenResponse, error) {
	// 1. Get the application URL for the file's mimetype.
	//This is going to be the "Bring Your Own App registry" service in Reva,
	//for now it's implemented by the wopiserver at /wopi/cbox/endpoints.
	// 2. Call the wopiserver's /wopi/cbox/open and pass the requested arguments
	// (documented at https://github.com/cs3org/wopiserver/blob/master/src/wopiserver.py#L238).
	// 3. Output all as the iframe_url field of the OpenResponse.
	//For reference, https://github.com/cs3org/wopiserver/blob/master/tools/wopiopen.py does a similar job

	// 1. applicationURL = GET /wopi/cbox/endpoints
	// # get the application server URLs
	wopiurl := ""   //TODO!
	iopsecret := "" //TODO!
	filename := ""
	eos := ""
	//res, err := http.Get(wopiurl + "/wopi/cbox/endpoints")

	var canedit bool

	if req.GetViewMode == providerpb.OpenRequest.ViewMode.VIEW_MODE_READ_WRITE {
		canedit = true
	} else {
		canedit = false
	}

	//2. tagert, accessToken = WOPI open(userId,canedit, username (optional), filename/fileid, folderUrl, endpoint(optional))
	// def cboxOpen():
	// '''Generates a WOPISrc target and an access token to be passed to a WOPI-compatible Office-like app
	// for accessing a given file for a given user.
	// Request arguments:
	// - string userid: user identity, typically an x-access-token;
	//   - OR int ruid, rgid: a real Unix user identity (id:group); this is for legacy compatibility
	// - bool canedit: True if full access should be given to the user, otherwise read-only access is granted
	// - string username (optional): user's full name, typically shown by the Office app
	// - string filename OR fileid: the full path of the filename to be opened, or its fileid
	// - string folderurl: the URL to come back to the containing folder for this file, typically shown by the Office app
	// - string endpoint (optional): the storage endpoint to be used to look up the file or the storage id, in case of
	//   multi-instance underlying storage; defaults to 'default'

	// client := &http.Client{
	// 	CheckRedirect: redirectPolicyFunc,
	// }
	httpClient := rhttp.GetHTTPClient(ctx)

	// params := url.Values{
	// 	"grant_type": {"client_credentials"},
	// 	"audience":   {m.conf.TargetAPI},
	// }
	foourl := ""
	params := url.Values{"x-access-token": {req.AccessToken}, "filename": {filename}, "endpoint": {eos},
		"canedit": {"true"}, "username": {"Operator"}, "folderurl": {"foo"}}
	httpReq, err := rhttp.NewRequest(ctx, "GET", foourl, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+iopsecret)

	httpRes, err := httpClient.Do(httpReq)
	// resp, err := client.Get("http://example.com")
	// // ...

	// req, err := http.NewRequest("GET", "http://example.com", nil)
	// // ...

	//wopisrc = http.Get(wopiurl + "/wopi/cbox/open",
	//httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("error performing http request")
		// w.WriteHeader(http.StatusInternalServerError)
		// return
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		log.Error().Err(err).Msg("error performing http request")
		// w.WriteHeader(http.StatusInternalServerError)
		// return
	}
	// if httpRes.StatusCode != http.StatusOK {
	// 	print("WOPI open request failed:\n%s" % httpRes.Body)
	// 	//	sys.exit(-1)
	// }

	//# return the full URL to the user
	// try:
	// 	url = apps[os.path.splitext(filename)[1]]["edit"]
	// 	url += '?'                              //if '?' not in url else '&'
	// 	print("App URL:\n%sWOPISrc=%s\n" + url) // % (url, httpRes.content.decode()))

	iframeLocation, err := s.provider.GetIFrame(ctx, req.Ref.GetId(), req.AccessToken)
	if err != nil {
		err := errors.Wrap(err, "appprovidersvc: error calling GetIFrame")
		res := &providerpb.OpenResponse{
			Status: status.NewInternal(ctx, err, "error getting app's iframe"),
		}
		return res, nil
	}
	res1 := &providerpb.OpenResponse{
		Status:    status.NewOK(ctx),
		IframeUrl: iframeLocation,
	}
	return res1, nil
}
