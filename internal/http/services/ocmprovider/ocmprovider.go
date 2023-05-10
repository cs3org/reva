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

package ocmprovider

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/go-chi/chi/v5"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("ocmprovider", New)
}

type config struct {
	OCMPrefix    string `mapstructure:"ocm_prefix"`
	Endpoint     string `mapstructure:"endpoint"`
	Provider     string `mapstructure:"provider"`
	WebDAVRoot   string `mapstructure:"webdav_root"`
	WebAppRoot   string `mapstructure:"webapp_root"`
	EnableWebApp bool   `mapstructure:"enable_webapp"`
	EnableDataTx bool   `mapstructure:"enable_datatx"`
}

type svc struct {
	Conf   *config
	router chi.Router
}

type discoveryData struct {
	Enabled       bool            `json:"enabled" xml:"enabled"`
	APIVersion    string          `json:"apiVersion" xml:"apiVersion"`
	Endpoint      string          `json:"endPoint" xml:"endPoint"`
	Provider      string          `json:"provider" xml:"provider"`
	ResourceTypes []resourceTypes `json:"resourceTypes" xml:"resourceTypes"`
	Capabilities  []string        `json:"capabilities" xml:"capabilities"`
}

type resourceTypes struct {
	Name       string            `json:"name"`
	ShareTypes []string          `json:"shareTypes"`
	Protocols  map[string]string `json:"protocols"`
}

type discoHandler struct {
	d discoveryData
}

func (c *config) init() {
	if c.OCMPrefix == "" {
		c.OCMPrefix = "ocm"
	}
	if c.Endpoint == "" {
		c.Endpoint = "http://localhost"
	}
	if c.Provider == "" {
		c.Provider = "reva"
	}
	if c.WebDAVRoot == "" {
		c.WebDAVRoot = "remote.php/dav"
	}
	if c.WebAppRoot == "" {
		c.WebAppRoot = "external/sciencemesh"
	}
}

func (h *discoHandler) init(c *config) {
	h.d.Enabled = true
	h.d.APIVersion = "1.1.0"
	h.d.Endpoint = fmt.Sprintf("%s/%s", c.Endpoint, c.OCMPrefix)
	h.d.Provider = c.Provider
	rtProtos := map[string]string{}
	// webdav is always enabled
	rtProtos["webdav"] = fmt.Sprintf("%s/%s/%s", c.Endpoint, c.WebDAVRoot, c.OCMPrefix)
	if c.EnableWebApp {
		rtProtos["webapp"] = fmt.Sprintf("%s/%s", c.Endpoint, c.WebAppRoot)
	}
	if c.EnableDataTx {
		rtProtos["datatx"] = fmt.Sprintf("%s/%s/%s", c.Endpoint, c.WebDAVRoot, c.OCMPrefix)
	}
	h.d.ResourceTypes = []resourceTypes{{
		Name:       "file",           // so far we only support `file`
		ShareTypes: []string{"user"}, // so far we only support `user`
		Protocols:  rtProtos,         // expose the protocols as per configuration
	}}
	// for now we hardcode the capabilities, as this is currently only advisory
	h.d.Capabilities = []string{"/invite-accepted"}
}

// New returns a new ocmprovider object, that implements
// the OCM discovery endpoint specified in
// https://cs3org.github.io/OCM-API/docs.html?repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	conf.init()

	r := chi.NewRouter()
	s := &svc{
		Conf:   conf,
		router: r,
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit() error {
	discoHandler := new(discoHandler)
	discoHandler.init(s.Conf)
	s.router.Get(".", discoHandler.Send)
	return nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	// this is hardcoded as per OCM specifications
	return "/ocm-provider"
}

func (s *svc) Unprotected() []string {
	return []string{"."}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Str("path", r.URL.Path).Msg("ocm-provider routing")

		// unset raw path, otherwise chi may use it to route and then failto match percent encoded path segments
		r.URL.RawPath = ""
		s.router.ServeHTTP(w, r)
	})
}

// Send sends the discovery info to the caller.
func (h *discoHandler) Send(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	indentedConf, _ := json.MarshalIndent(h.d, "", "   ")
	if _, err := w.Write(indentedConf); err != nil {
		log.Err(err).Msg("Error writing to ResponseWriter")
	}
}
