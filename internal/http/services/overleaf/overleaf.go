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
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storagepb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

type svc struct {
	conf      *config
	gtwClient gateway.GatewayAPIClient
	log       *zerolog.Logger
	router    *chi.Mux
}

type config struct {
	Prefix      string `mapstructure:"prefix"`
	GatewaySvc  string `mapstructure:"gatewaysvc"`
	AppName     string `mapstructure:"app_name" docs:";The App user-friendly name."`
	ArchiverURL string `mapstructure:"archiver_url" docs:";Internet-facing URL of the archiver service, used to serve the files to Overleaf."`
	AppURL      string `mapstructure:"app_url" docs:";The App URL."`
	Insecure    bool   `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
}

func init() {
	global.Register("overleaf", New)
}

func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint(conf.GatewaySvc))
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()

	s := &svc{
		conf:      conf,
		gtwClient: gtw,
		router:    r,
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit() error {
	s.router.Get("/import", s.handleImport)
	s.router.Post("/export", s.handleExport)
	return nil
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "overleaf"
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.router.ServeHTTP(w, r)
	})
}

func (s *svc) handleImport(w http.ResponseWriter, r *http.Request) {
	reqres.WriteError(w, r, reqres.APIErrorUnimplemented, "Overleaf import not yet supported", nil)
}

func (s *svc) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	statRes, err := s.validateQuery(w, r, ctx)
	if err != nil {
		// Validate query handles errors
		return
	}

	resource := statRes.Info

	// User needs to have download rights to export to Overleaf
	if !resource.PermissionSet.InitiateFileDownload {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "permission denied when accessing the file", err)
		return
	}

	if resource.Type != storagepb.ResourceType_RESOURCE_TYPE_FILE && resource.Type != storagepb.ResourceType_RESOURCE_TYPE_CONTAINER {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "invalid resource type, resource should be a file or a folder", nil)
		return
	}

	token, ok := ctxpkg.ContextGetToken(ctx)
	if !ok || token == "" {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "Access token is invalid or empty", err)
		return
	}

	if r.Form.Get("override") == "" {
		creationTime, alreadySet := resource.GetArbitraryMetadata().Metadata["reva.overleaf.exporttime"]
		if alreadySet {
			w.WriteHeader(http.StatusConflict)
			if err := json.NewEncoder(w).Encode(map[string]any{
				"code":        "ALREADY_EXISTS",
				"message":     "Project was already exported",
				"export_time": creationTime,
			}); err != nil {
				reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling JSON response", err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			return
		}
	}

	// TODO: generate and use a more restricted token
	restrictedToken := token

	// Setting up archiver request
	archHTTPReq, err := rhttp.NewRequest(ctx, http.MethodGet, s.conf.ArchiverURL, nil)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "overleaf: error setting up http request", nil)
		return
	}

	archQuery := archHTTPReq.URL.Query()
	archQuery.Add("id", resource.Id.StorageId+"!"+resource.Id.OpaqueId)
	archQuery.Add("access_token", restrictedToken)
	archQuery.Add("arch_type", "zip")

	archHTTPReq.URL.RawQuery = archQuery.Encode()
	log.Debug().Str("Archiver url", archHTTPReq.URL.String()).Msg("URL for downloading zipped resource from archiver")

	// Setting up Overleaf request
	appUrl := s.conf.AppURL + "/docs"
	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, appUrl, nil)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "overleaf: error setting up http request", nil)
		return
	}

	q := httpReq.URL.Query()

	// snip_uri is link to archiver request
	q.Add("snip_uri", archHTTPReq.URL.String())

	// getting file/folder name so as not to expose authentication token in project name
	name := strings.TrimSuffix(filepath.Base(resource.Path), filepath.Ext(resource.Path))
	q.Add("snip_name", name)

	httpReq.URL.RawQuery = q.Encode()
	url := httpReq.URL.String()

	req := &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{
			ResourceId: resource.Id,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"reva.overleaf.exporttime": strconv.Itoa(int(time.Now().Unix())),
				"reva.overleaf.name":       base64.StdEncoding.EncodeToString([]byte(name)),
			},
		},
	}

	res, err := s.gtwClient.SetArbitraryMetadata(ctx, req)

	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "overleaf: error setting arbitrary metadata", nil)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "overleaf: error statting", nil)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"app_url": url,
	}); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling JSON response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (s *svc) validateQuery(w http.ResponseWriter, r *http.Request, ctx context.Context) (*storagepb.StatResponse, error) {
	if err := r.ParseForm(); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "parameters could not be parsed", nil)
		return nil, err
	}

	resourceID := r.Form.Get("resource_id")

	var resourceRef storagepb.Reference
	if resourceID == "" {
		path := r.Form.Get("path")
		if path == "" {
			reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "missing resource ID or path", nil)
			return nil, errors.New("missing resource ID or path")
		}
		resourceRef.Path = path
	} else {
		resourceID := resourceid.OwnCloudResourceIDUnwrap(resourceID)
		if resourceID == nil {
			reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "invalid resource ID", nil)
			return nil, errors.New("invalid resource ID")
		}
		resourceRef.ResourceId = resourceID
	}

	statRes, err := s.gtwClient.Stat(ctx, &storagepb.StatRequest{Ref: &resourceRef})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "Internal error accessing the resource, please try again later", err)
		return nil, errors.New("Internal error accessing the resource, please try again later")
	}

	if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
		reqres.WriteError(w, r, reqres.APIErrorNotFound, "resource does not exist", nil)
		return nil, errors.New("resource does not exist")
	} else if statRes.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "failed to stat the resource", nil)
		return nil, errors.New("failed to stat the resource")
	}

	return statRes, nil
}
