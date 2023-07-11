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

package preferences

import (
	"context"
	"encoding/json"
	"net/http"

	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "preferences"

func init() {
	global.Register(name, New)
}

// Config holds the config options that for the preferences HTTP service.
type Config struct {
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

func (c *Config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf *Config
}

// New returns a new ocmd object.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		conf: &c,
	}

	return s, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Register(r mux.Router) {
	r.Route("/preferences", func(r mux.Router) {
		r.Get("/", http.HandlerFunc(s.handleGet))
		r.Post("/", http.HandlerFunc(s.handlePost))
	})
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) handleGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	key := r.URL.Query().Get("key")
	ns := r.URL.Query().Get("ns")

	if key == "" || ns == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("key or namespace query missing")); err != nil {
			log.Error().Err(err).Msg("error writing to response")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := client.GetKey(ctx, &preferences.GetKeyRequest{
		Key: &preferences.PreferenceKey{
			Namespace: ns,
			Key:       key,
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error retrieving key")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		log.Error().Interface("status", res.Status).Msg("error retrieving key")
		return
	}

	js, err := json.Marshal(map[string]interface{}{
		"namespace": ns,
		"key":       key,
		"value":     res.Val,
	})
	if err != nil {
		log.Error().Err(err).Msg("error marshalling response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Error().Err(err).Msg("error writing JSON response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *svc) handlePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	key := r.FormValue("key")
	ns := r.FormValue("ns")
	val := r.FormValue("value")

	if key == "" || ns == "" || val == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("key, namespace or value parameter missing")); err != nil {
			log.Error().Err(err).Msg("error writing to response")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := client.SetKey(ctx, &preferences.SetKeyRequest{
		Key: &preferences.PreferenceKey{
			Namespace: ns,
			Key:       key,
		},
		Val: val,
	})
	if err != nil {
		log.Error().Err(err).Msg("error setting key")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		log.Error().Interface("status", res.Status).Msg("error setting key")
		return
	}
}
