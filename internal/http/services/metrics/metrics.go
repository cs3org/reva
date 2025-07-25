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

package metrics

/*
This service initializes the metrics package according to the metrics configuration.
*/
import (
	"context"
	"net/http"
	"os"

	"github.com/cs3org/reva/v3/pkg/logger"
	"github.com/cs3org/reva/v3/pkg/metrics"
	"github.com/cs3org/reva/v3/pkg/metrics/config"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	global.Register(serviceName, New)
}

const (
	serviceName = "metrics"
)

// Close is called when this service is being stopped.
func (s *svc) Close() error {
	return nil
}

// Prefix returns the main endpoint of this service.
func (s *svc) Prefix() string {
	// We use a dummy endpoint as the service is not expected to be exposed
	// directly to the user, but just start a background process.
	return "register_metrics"
}

// Unprotected returns all endpoints that can be queried without prior authorization.
func (s *svc) Unprotected() []string {
	return []string{}
}

// Handler serves all HTTP requests.
func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logger.New().With().Int("pid", os.Getpid()).Logger()
		if _, err := w.Write([]byte("This is the metrics service.\n")); err != nil {
			log.Error().Err(err).Msg("error writing metrics response")
		}
	})
}

// New returns a new metrics service.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config.Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// initialize metrics using the configuration
	err := metrics.Init(&c)
	if err != nil {
		return nil, err
	}

	// Create the service
	s := &svc{}
	return s, nil
}

type svc struct {
}
