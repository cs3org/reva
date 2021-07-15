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

package ocmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/ocmd"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("appprovider", New)
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix         string `mapstructure:"prefix"`
	GatewaySvc     string `mapstructure:"gatewaysvc"`
	AccessTokenTTL int    `mapstructure:"access_token_ttl"`
}

func (c *Config) init() {
	if c.Prefix == "" {
		c.Prefix = "api/v0/wopi/open"
	}
	if c.AccessTokenTTL == 0 {
		c.AccessTokenTTL = 86400
	}
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf *Config
}

// New returns a new ocmd object
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {

	conf := &Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	conf.init()

	s := &svc{
		conf: conf,
	}
	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			ocmd.WriteError(w, r, ocmd.APIErrorUnimplemented, "only GET requests are supported", errors.New("only GET requests are supported"))
			return
		}

		s.handleWopiOpen(w, r)
	})
}

// WopiResponse holds the various fields to be returned for a wopi open call
type WopiResponse struct {
	WopiClientURL  string `json:"wopiclienturl"`
	AccessToken    string `json:"accesstoken"`
	AccessTokenTTL int64  `json:"accesstokenttl"`
}

func (s *svc) handleWopiOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(s.conf.GatewaySvc)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error getting grpc gateway client", err)
		return
	}

	info, errCode, err := s.getStatInfo(ctx, r.URL.Query().Get("fileId"), client)
	if err != nil {
		ocmd.WriteError(w, r, errCode, "error statting file", err)
	}

	openReq := gateway.OpenInAppRequest{
		Ref:      &provider.Reference{ResourceId: info.Id},
		ViewMode: getViewMode(info),
		App:      r.URL.Query().Get("app"),
	}
	openRes, err := client.OpenInApp(ctx, &openReq)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error opening resource", err)
		return
	}
	if openRes.Status.Code != rpc.Code_CODE_OK {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error opening resource information", status.NewErrorFromCode(openRes.Status.Code, "appprovider"))
		return
	}

	u, err := url.Parse(openRes.AppUrl)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error parsing app URL", err)
		return
	}
	q := u.Query()

	// remove access token from query parameters
	accessToken := q.Get("access_token")
	q.Del("access_token")

	// more options used by oC 10:
	// &lang=en-GB
	// &closebutton=1
	// &revisionhistory=1
	// &title=Hello.odt
	u.RawQuery = q.Encode()

	js, err := json.Marshal(
		WopiResponse{
			WopiClientURL: u.String(),
			AccessToken:   accessToken,
			// https://wopi.readthedocs.io/projects/wopirest/en/latest/concepts.html#term-access-token-ttl
			AccessTokenTTL: time.Now().Add(time.Second*time.Duration(s.conf.AccessTokenTTL)).UnixNano() / 1e6,
		},
	)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error marshalling JSON response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error writing JSON response", err)
		return
	}
}

func (s *svc) getStatInfo(ctx context.Context, fileID string, client gateway.GatewayAPIClient) (*provider.ResourceInfo, ocmd.APIErrorCode, error) {
	if fileID == "" {
		return nil, ocmd.APIErrorInvalidParameter, errors.New("fileID parameter missing in request")
	}

	decodedID, err := base64.URLEncoding.DecodeString(fileID)
	if err != nil {
		return nil, ocmd.APIErrorInvalidParameter, errors.Wrap(err, "fileID doesn't follow the required format")
	}

	parts := strings.Split(string(decodedID), ":")
	if !utf8.ValidString(parts[0]) || !utf8.ValidString(parts[1]) {
		return nil, ocmd.APIErrorInvalidParameter, errors.New("fileID contains illegal characters")
	}
	res := &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}

	statReq := provider.StatRequest{
		Ref: &provider.Reference{ResourceId: res},
	}
	statRes, err := client.Stat(ctx, &statReq)
	if err != nil {
		return nil, ocmd.APIErrorServerError, err
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		return nil, ocmd.APIErrorServerError, status.NewErrorFromCode(statRes.Status.Code, "appprovider")
	}
	if statRes.Info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		return nil, ocmd.APIErrorServerError, errors.New("unsupported resource type")
	}

	return statRes.Info, ocmd.APIErrorCode(""), nil
}

func getViewMode(res *provider.ResourceInfo) gateway.OpenInAppRequest_ViewMode {
	var viewMode gateway.OpenInAppRequest_ViewMode
	canEdit := res.PermissionSet.InitiateFileUpload
	canView := res.PermissionSet.InitiateFileDownload

	switch {
	case canEdit && canView:
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE
	case canView:
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY
	default:
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_INVALID
	}
	return viewMode
}
