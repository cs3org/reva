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

package blobstore

import (
	"github.com/prometheus/client_golang/prometheus"
	"go-micro.dev/v4/util/log"
)

var (
	// Namespace defines the namespace for the defines metrics.
	Namespace = "ocis"

	// Subsystem defines the subsystem for the defines metrics.
	Subsystem = "s3ng"
)

// Metrics defines the available metrics of this service.
type Metrics struct {
	Rx *prometheus.CounterVec
	Tx *prometheus.CounterVec
}

// NewMetrics initializes the available metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		Rx: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "Rx",
			Help:      "Storage access rx",
		}, []string{}),
		Tx: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "Tx",
			Help:      "Storage access tx",
		}, []string{}),
	}
	err := prometheus.Register(m.Rx)
	if err != nil {
		log.Errorf("Failed to register prometheus storage rx Counter (%s)", err)
	}
	err = prometheus.Register(m.Tx)
	if err != nil {
		log.Errorf("Failed to register prometheus storage tx Counter (%s)", err)
	}

	return m
}
