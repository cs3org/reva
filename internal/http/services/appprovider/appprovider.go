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

package appprovider

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"

	apppb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagepb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/cs3org/reva/v3/pkg/utils/resourceid"
	"github.com/go-chi/chi/v5"
	ua "github.com/mileusna/useragent"
	"github.com/pkg/errors"
)

func init() {
	global.Register("appprovider", New)
}

// Config holds the config options for the HTTP appprovider service.
type Config struct {
	Prefix     string `mapstructure:"prefix"`
	GatewaySvc string `mapstructure:"gatewaysvc"                                              validate:"required"`
	Insecure   bool   `docs:"false;Whether to skip certificate checks when sending requests." mapstructure:"insecure"`
}

func (c *Config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "app"
	}
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf   *Config
	router *chi.Mux
}

// New returns a new ocmd object.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	s := &svc{
		conf:   &c,
		router: r,
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit() error {
	s.router.Get("/list", s.handleList)
	s.router.Post("/new", s.handleNew)
	s.router.Post("/open", s.handleOpen)
	s.router.Post("/notify", s.handleNotify)
	return nil
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
		s.router.ServeHTTP(w, r)
	})
}

func (s *svc) handleNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		writeError(w, r, appErrorServerError, "error getting grpc gateway client", err)
		return
	}

	err = r.ParseForm()
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, "parameters could not be parsed", nil)
	}

	if r.Form.Get("template") != "" {
		// TODO in the future we want to create a file out of the given template
		writeError(w, r, appErrorUnimplemented, "template is not implemented", nil)
		return
	}

	parentContainerID := r.Form.Get("parent_container_id")
	if parentContainerID == "" {
		writeError(w, r, appErrorInvalidParameter, "missing parent container ID", nil)
		return
	}

	parentContainerRef, ok := spaces.ParseResourceID(parentContainerID)
	if !ok {
		// If this fails, client might be non-spaces
		var err error
		parentContainerRef, err = spaces.ResourceIdFromString(parentContainerID)
		if err != nil {
			writeError(w, r, appErrorInvalidParameter, "invalid parent container ID", nil)
			return
		}
	}

	filename := r.Form.Get("filename")
	if filename == "" {
		writeError(w, r, appErrorInvalidParameter, "missing filename", nil)
		return
	}

	dirPart, filePart := path.Split(filename)
	if dirPart != "" || filePart != filename {
		writeError(w, r, appErrorInvalidParameter, "the filename must not contain a path segment", nil)
		return
	}

	statParentContainerReq := &storagepb.StatRequest{
		Ref: &storagepb.Reference{
			ResourceId: parentContainerRef,
		},
	}
	parentContainer, err := client.Stat(ctx, statParentContainerReq)
	if err != nil {
		writeError(w, r, appErrorServerError, "error sending a grpc stat request", err)
		return
	}

	if parentContainer.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorNotFound, "the parent container is not accessible or does not exist", err)
		return
	}

	if parentContainer.Info.Type != storagepb.ResourceType_RESOURCE_TYPE_CONTAINER {
		writeError(w, r, appErrorInvalidParameter, "the parent container id does not point to a container", nil)
		return
	}

	fileRef := &storagepb.Reference{
		Path: path.Join(parentContainer.Info.Path, utils.MakeRelativePath(filename)),
	}

	statFileReq := &storagepb.StatRequest{
		Ref: fileRef,
	}
	statFileRes, err := client.Stat(ctx, statFileReq)
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to stat the file", err)
		return
	}

	if statFileRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		if statFileRes.Status.Code == rpc.Code_CODE_OK {
			writeError(w, r, appErrorAlreadyExists, "the file already exists", nil)
			return
		}
		writeError(w, r, appErrorServerError, "statting the file returned unexpected status code", err)
		return
	}

	// Create empty file via storageprovider
	createReq := &storagepb.InitiateFileUploadRequest{
		Ref: fileRef,
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				ocdav.HeaderUploadLength: {
					Decoder: "plain",
					Value:   []byte("0"),
				},
			},
		},
	}

	// having a client.CreateFile() function would come in handy here...

	createRes, err := client.InitiateFileUpload(ctx, createReq)
	if err != nil {
		writeError(w, r, appErrorServerError, "error calling InitiateFileUpload", err)
		return
	}
	if createRes.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, "error calling InitiateFileUpload", nil)
		return
	}

	// Do a HTTP PUT with an empty body
	var ep, token string
	for _, p := range createRes.Protocols {
		if p.Protocol == "simple" {
			ep, token = p.UploadEndpoint, p.Token
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, ep, nil)
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to create the file", err)
		return
	}

	httpReq.Header.Set(datagateway.TokenTransportHeader, token)
	httpReq.Header.Set(ocdav.HeaderContentLength, strconv.Itoa(0))
	httpReq.Header.Set(ocdav.HeaderUploadLength, strconv.Itoa(0))

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: s.conf.Insecure}}
	httpRes, err := httpclient.New(httpclient.RoundTripper(tr)).Do(httpReq)
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to create the file", err)
		return
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode == http.StatusForbidden {
		// the file upload was already finished since it is a zero byte file
		// TODO: why do we get a 401 then!?
	} else if httpRes.StatusCode != http.StatusOK {
		writeError(w, r, appErrorServerError, "failed to create the file", nil)
		return
	}

	// Stat the newly created file
	statRes, err := client.Stat(ctx, statFileReq)
	if err != nil {
		writeError(w, r, appErrorServerError, "statting the created file failed", err)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, "statting the created file failed", nil)
		return
	}

	if statRes.Info.Type != storagepb.ResourceType_RESOURCE_TYPE_FILE {
		writeError(w, r, appErrorInvalidParameter, "the given file id does not point to a file", nil)
		return
	}

	js, err := json.Marshal(
		map[string]interface{}{
			"file_id": spaces.EncodeResourceID(statRes.Info.Id),
		},
	)
	if err != nil {
		writeError(w, r, appErrorServerError, "error marshalling JSON response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		writeError(w, r, appErrorServerError, "error writing JSON response", err)
		return
	}
}

func (s *svc) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		writeError(w, r, appErrorServerError, "error getting grpc gateway client", err)
		return
	}

	listRes, err := client.ListSupportedMimeTypes(ctx, &appregistry.ListSupportedMimeTypesRequest{})
	if err != nil {
		writeError(w, r, appErrorServerError, "error listing supported mime types", err)
		return
	}
	if listRes.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, "error listing supported mime types", nil)
		return
	}

	res := filterAppsByUserAgent(listRes.MimeTypes, r.UserAgent())
	js, err := json.Marshal(map[string]interface{}{"mime-types": res})
	if err != nil {
		writeError(w, r, appErrorServerError, "error marshalling JSON response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		writeError(w, r, appErrorServerError, "error writing JSON response", err)
		return
	}
}

func (s *svc) handleOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		writeError(w, r, appErrorServerError, "Internal error with the gateway, please try again later", err)
		return
	}

	err = r.ParseForm()
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, "parameters could not be parsed", nil)
	}

	fileID := r.Form.Get("file_id")

	var fileRef storagepb.Reference
	if fileID == "" {
		path := r.Form.Get("path")
		if path == "" {
			writeError(w, r, appErrorInvalidParameter, "missing file ID or path", nil)
			return
		}
		fileRef.Path = path
	} else {
		resourceID, ok := spaces.ParseResourceID(fileID)
		if !ok {
			// we try to fall back for non-spaces requests
			resourceID = resourceid.OwnCloudResourceIDUnwrap(fileID)
			if resourceID == nil {
				writeError(w, r, appErrorInvalidParameter, "invalid file ID", nil)
				return
			}
		}
		fileRef.ResourceId = resourceID
	}

	statRes, err := client.Stat(ctx, &storagepb.StatRequest{Ref: &fileRef})
	if err != nil {
		writeError(w, r, appErrorServerError, "Internal error accessing the file, please try again later", err)
		return
	}

	if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
		writeError(w, r, appErrorNotFound, "file does not exist", nil)
		return
	} else if statRes.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, "failed to stat the file", nil)
		return
	}

	if statRes.Info.Type != storagepb.ResourceType_RESOURCE_TYPE_FILE && statRes.Info.Type != storagepb.ResourceType_RESOURCE_TYPE_CONTAINER {
		writeError(w, r, appErrorInvalidParameter, "the given file id does not point to a file or a container", nil)
		return
	}

	viewMode := resolveViewMode(statRes.Info, r.Form.Get("view_mode"))
	if viewMode == gateway.OpenInAppRequest_VIEW_MODE_INVALID {
		writeError(w, r, appErrorUnauthenticated, "permission denied when accessing the file", err)
		return
	}

	opaqueMap := make(map[string]*typespb.OpaqueEntry)
	for k, v := range r.Form {
		if k != "file_id" && k != "view_mode" && k != "app_name" {
			opaqueMap[k] = &typespb.OpaqueEntry{
				Decoder: "plain",
				Value:   []byte(v[0]),
			}
		}
	}
	appName := r.Form.Get("app_name")
	appName, err = url.QueryUnescape(appName)
	if err != nil {
		writeError(w, r, appErrorServerError,
			"Error decoding app_name", err)
		return
	}

	openReq := gateway.OpenInAppRequest{
		Ref:      &fileRef,
		ViewMode: viewMode,
		App:      appName,
		Opaque:   &typespb.Opaque{Map: opaqueMap},
	}
	openRes, err := client.OpenInApp(ctx, &openReq)
	if err != nil {
		writeError(w, r, appErrorServerError,
			"Error contacting the requested application, please use a different one or try again later", err)
		return
	}
	if openRes.Status.Code != rpc.Code_CODE_OK {
		if openRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			writeError(w, r, appErrorNotFound, openRes.Status.Message, nil)
			return
		}
		if openRes.Status.Code == rpc.Code_CODE_ALREADY_EXISTS {
			writeError(w, r, appErrorAlreadyExists, openRes.Status.Message, nil)
			return
		}
		writeError(w, r, appErrorServerError, openRes.Status.Message,
			status.NewErrorFromCode(openRes.Status.Code, "error calling OpenInApp"))
		return
	}

	// recreate the structure to be able to marshal the AppUrl.Target as a string
	js, err := json.Marshal(
		map[string]interface{}{
			"app_url":         openRes.AppUrl.AppUrl,
			"method":          openRes.AppUrl.Method,
			"form_parameters": openRes.AppUrl.FormParameters,
			"headers":         openRes.AppUrl.Headers,
			"target":          appTargetToString(openRes.AppUrl.Target),
		},
	)
	if err != nil {
		writeError(w, r, appErrorServerError, "Internal error with JSON payload",
			errors.Wrap(err, "error marshalling JSON response"))
		return
	}

	log := appctx.GetLogger(ctx)
	log.Info().Interface("resource", fileRef.ResourceId).
		Str("url", openRes.AppUrl.AppUrl).
		Str("method", openRes.AppUrl.Method).
		Interface("viewMode", viewMode).
		Str("fileExt", filepath.Ext(statRes.Info.Path)).
		Str("agent", utils.SimplifiedUserAgent(r)).
		Msg("returning app URL for file")

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		writeError(w, r, appErrorServerError, "Internal error with JSON payload",
			errors.Wrap(err, "error writing JSON response"))
		return
	}
}

func (s *svc) handleNotify(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, "parameters could not be parsed", nil)
	}

	fileID := r.Form.Get("file_id")
	var fileRef storagepb.Reference
	if fileID == "" {
		path := r.Form.Get("path")
		if path == "" {
			writeError(w, r, appErrorInvalidParameter, "missing file ID or path", nil)
			return
		}
		fileRef.Path = path
	} else {
		resourceID, ok := spaces.ParseResourceID(fileID)
		if !ok {
			// If this fails, client might be non-spaces
			var err error
			resourceID, err = spaces.ResourceIdFromString(fileID)
			if err != nil {
				writeError(w, r, appErrorInvalidParameter, "invalid file ID", nil)
				return
			}
		}
		fileRef.ResourceId = resourceID
	}

	// the body of the request may contain any error the client got when attempting to open the app
	failure, _ := io.ReadAll(r.Body)

	// log the fileid for later correlation / monitoring
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	if len(failure) == 0 {
		log.Info().Interface("resource", fileRef.ResourceId).Msg("file successfully opened in app")
	} else {
		log.Info().Interface("resource", fileRef.ResourceId).Str("failure", string(failure)).Msg("failed to open file in app")
	}

	w.WriteHeader(http.StatusOK)
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
			m.AppProviders = apps
			res = append(res, m)
		}
	}
	return res
}

func resolveViewMode(res *storagepb.ResourceInfo, vm string) gateway.OpenInAppRequest_ViewMode {
	var viewMode gateway.OpenInAppRequest_ViewMode
	if vm != "" {
		viewMode = utils.GetViewMode(vm)
	} else {
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE
	}
	canEdit := res.PermissionSet.InitiateFileUpload
	canView := res.PermissionSet.InitiateFileDownload

	switch {
	case canEdit && canView:
		// ok
	case canView:
		if viewMode == gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE || viewMode == gateway.OpenInAppRequest_VIEW_MODE_PREVIEW {
			// downgrade to the maximum permitted viewmode
			viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY
		}
	default:
		// no permissions, will return access denied
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_INVALID
	}
	return viewMode
}

func appTargetToString(t apppb.Target) string {
	switch t {
	case apppb.Target_TARGET_IFRAME:
		return "iframe"
	case apppb.Target_TARGET_BLANK:
		return "blank"
	default:
		return "invalid"
	}
}
