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

package prometheus

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/metrics"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opencensus.io/stats/view"
)

func init() {
	global.Register("prometheus", New)
	fmt.Printf("metrics - %v \n", metrics.NumUsersMeasure.Description())
}

// New returns a new prometheus service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.Prefix == "" {
		conf.Prefix = "metrics"
	}

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "revad",
	})
	if err != nil {
		return nil, errors.Wrap(err, "prometheus: error creating exporter")
	}

	view.RegisterExporter(pe)

	// register the desired measures' views
	if err = view.Register(
		metrics.GetNumUsersView(),
		metrics.GetNumGroupsView(),
		metrics.GetAmountStorageView(),
	); err != nil {
		return nil, errors.Wrap(err, "prometheus: error registering exporter")
	}

	return &svc{prefix: conf.Prefix, h: pe}, nil
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix string
	h      http.Handler
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.h
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	// TODO(labkode): all prometheus endpoints are public?
	return []string{"/"}
}
