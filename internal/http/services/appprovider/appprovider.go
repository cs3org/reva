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

package appprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/internal/http/services/ocmd"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
	ua "github.com/mileusna/useragent"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	global.Register("appprovider", New)
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix     string `mapstructure:"prefix"`
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

func (c *Config) init() {
	if c.Prefix == "" {
		c.Prefix = "app"
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
	return []string{"/list"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)

		switch head {
		case "new":
			s.handleNew(w, r)
		case "list":
			s.handleList(w, r)
		case "open":
			s.handleOpen(w, r)
		}
	})
}

func (s *svc) handleNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(s.conf.GatewaySvc)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error getting grpc gateway client", err)
		return
	}

	if r.URL.Query().Get("template") != "" {
		// TODO in the future we want to create a file out of the given template
		ocmd.WriteError(w, r, ocmd.APIErrorInvalidParameter, "Template not implemented",
			errtypes.NotSupported("Templates are not yet supported"))
		return
	}

	target := r.URL.Query().Get("filename")
	if target == "" {
		ocmd.WriteError(w, r, ocmd.APIErrorInvalidParameter, "Missing filename",
			errtypes.UserRequired("Missing filename"))
		return
	}
	// stat the container
	_, ocmderr, err := statRef(ctx, provider.Reference{Path: r.URL.Query().Get("container")}, client)
	if err != nil {
		log.Error().Err(err).Msg("error statting container")
		ocmd.WriteError(w, r, ocmderr, "Container not found", errtypes.NotFound("Container not found"))
		return
	}
	// Create empty file via storageprovider: obtain the HTTP URL for a PUT
	target = path.Join(r.URL.Query().Get("container"), target)
	createReq := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: target},
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"Upload-Length": {
					Decoder: "plain",
					Value:   []byte(strconv.FormatInt(0, 10)),
				},
			},
		},
	}
	createRes, err := client.InitiateFileUpload(ctx, createReq)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error calling InitiateFileUpload", err)
		return
	}
	if createRes.Status.Code != rpc.Code_CODE_OK {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error creating resource", status.NewErrorFromCode(createRes.Status.Code, "appprovider"))
		return
	}

	// Do a HTTP PUT with an empty body
	var ep, token string
	for _, p := range createRes.Protocols {
		if p.Protocol == "simple" {
			ep, token = p.UploadEndpoint, p.Token
		}
	}
	httpReq, err := rhttp.NewRequest(ctx, http.MethodPut, ep, nil)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error executing PUT", err)
		return
	}

	httpReq.Header.Set(datagateway.TokenTransportHeader, token)
	httpRes, err := rhttp.GetHTTPClient().Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("error doing PUT request to data service")
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error executing PUT", err)
		return
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode != http.StatusOK {
		log.Error().Msg("PUT request to data server failed")
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error executing PUT",
			errtypes.InternalError(fmt.Sprint(httpRes.StatusCode)))
		return
	}

	// Stat created file and return its file id
	statRes, ocmderr, err := statRef(ctx, provider.Reference{Path: target}, client)
	if err != nil {
		log.Error().Err(err).Msg("error statting created file")
		ocmd.WriteError(w, r, ocmderr, "Created file not found", errtypes.NotFound("Created file not found"))
		return
	}
	js, err := json.Marshal(map[string]interface{}{"file_id": statRes.Id})
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

func (s *svc) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	client, err := pool.GetGatewayServiceClient(s.conf.GatewaySvc)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error getting grpc gateway client", err)
		return
	}

	listRes, err := client.ListSupportedMimeTypes(ctx, &appregistry.ListSupportedMimeTypesRequest{})
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error listing supported mime types", err)
		return
	}
	if listRes.Status.Code != rpc.Code_CODE_OK {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error listing supported mime types",
			status.NewErrorFromCode(listRes.Status.Code, "appprovider"))
		return
	}

	res := filterAppsByUserAgent(listRes.MimeTypes, r.UserAgent())
	js, err := json.Marshal(map[string]interface{}{"mime-types": res})
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

func (s *svc) handleOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(s.conf.GatewaySvc)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error getting grpc gateway client", err)
		return
	}

	info, errCode, err := s.getStatInfo(ctx, r.URL.Query().Get("file_id"), client)
	if err != nil {
		ocmd.WriteError(w, r, errCode, "error statting file", err)
		return
	}

	openReq := gateway.OpenInAppRequest{
		Ref:      &provider.Reference{ResourceId: info.Id},
		ViewMode: getViewMode(info, r.URL.Query().Get("view_mode")),
		App:      r.URL.Query().Get("app_name"),
	}
	openRes, err := client.OpenInApp(ctx, &openReq)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error opening resource", err)
		return
	}
	if openRes.Status.Code != rpc.Code_CODE_OK {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error opening resource information",
			status.NewErrorFromCode(openRes.Status.Code, "appprovider"))
		return
	}

	js, err := json.Marshal(openRes.AppUrl)
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

func filterAppsByUserAgent(mimeTypes []*appregistry.MimeTypeInfo, userAgent string) []*appregistry.MimeTypeInfo {
	ua := ua.Parse(userAgent)
	res := []*appregistry.MimeTypeInfo{}
	for _, m := range mimeTypes {
		apps := []*appregistry.ProviderInfo{}
		for _, p := range m.AppProviders {
			p.Address = "" // address is internal only and not needed in the client
			// apps are called by name, so if it has no name it cannot be called and should not be advertised
			// also filter Desktop-only apps if ua is not Desktop
			if p.Name != "" && (ua.Desktop || !p.DesktopOnly) {
				apps = append(apps, p)
			}
		}
		if len(apps) > 0 {
			res = append(res, m)
		}
	}
	return res
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

	return statRef(ctx, provider.Reference{ResourceId: res}, client)
}

func statRef(ctx context.Context, ref provider.Reference, client gateway.GatewayAPIClient) (*provider.ResourceInfo, ocmd.APIErrorCode, error) {
	statReq := provider.StatRequest{Ref: &ref}
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

func getViewMode(res *provider.ResourceInfo, vm string) gateway.OpenInAppRequest_ViewMode {
	if vm != "" {
		return utils.GetViewMode(vm)
	}

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
