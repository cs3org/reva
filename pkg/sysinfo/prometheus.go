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

package sysinfo

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cs3org/reva/pkg/utils"
)

// PrometheusExporter exports system information via Prometheus.
type PrometheusExporter struct {
	registry      *prometheus.Registry
	sysInfoMetric prometheus.GaugeFunc
}

func (psysinfo *PrometheusExporter) init(registry *prometheus.Registry) error {
	// Create all necessary Prometheus objects
	psysinfo.registry = registry
	psysinfo.sysInfoMetric = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace:   "revad",
			Name:        "sys_info",
			Help:        "A metric with a constant '1' value labeled by various system information elements",
			ConstLabels: psysinfo.getLabels("", SysInfo),
		},
		func() float64 { return 1 },
	)

	if err := psysinfo.registry.Register(psysinfo.sysInfoMetric); err != nil {
		return fmt.Errorf("unable to register the system information metrics: %v", err)
	}

	return nil
}

func (psysinfo *PrometheusExporter) getLabels(root string, i interface{}) prometheus.Labels {
	labels := prometheus.Labels{}

	// Iterate over each field of the given interface, recursively collecting the values as labels
	v := reflect.ValueOf(i).Elem()
	for i := 0; i < v.NumField(); i++ {
		// Check if the field was tagged with 'sysinfo:omitlabel'; if so, skip this field
		tags := v.Type().Field(i).Tag.Get("sysinfo")
		if strings.Contains(tags, "omitlabel") {
			continue
		}

		// Get the name of the field from the parent structure
		fieldName := utils.ToSnakeCase(v.Type().Field(i).Name)
		if len(root) > 0 {
			fieldName = "_" + fieldName
		}
		fieldName = root + fieldName

		// Check if the field is either a struct or a pointer to a struct; in that case, process the field recursively
		f := v.Field(i)
		if f.Kind() == reflect.Struct || (f.Kind() == reflect.Ptr && f.Elem().Kind() == reflect.Struct) {
			// Merge labels recursively
			for key, val := range psysinfo.getLabels(fieldName, f.Interface()) {
				labels[key] = val
			}
		} else { // Store the value of the field in the labels
			labels[fieldName] = fmt.Sprintf("%v", f)
		}
	}

	return labels
}

// NewPrometheusExporter creates a new Prometheus system information exporter.
func NewPrometheusExporter(registry *prometheus.Registry) (*PrometheusExporter, error) {
	if registry == nil {
		return nil, fmt.Errorf("no registry provided")
	}

	exporter := &PrometheusExporter{}
	if err := exporter.init(registry); err != nil {
		return nil, err
	}

	return exporter, nil
}
