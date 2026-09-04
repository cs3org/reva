// Copyright 2018-2026 CERN
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

package stats

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"

	"github.com/cs3org/reva/v3/pkg/stats"
	"github.com/prometheus/client_golang/prometheus"
)

// aggregator merges the payloads of a refresh into prometheus const
// metrics. Families with the same name are merged across services and
// payloads (e.g. "shares" from the user share and public link providers);
// identically-labelled samples sum (the per-instance fan-out case).
type aggregator struct {
	namespace string
	order     []string
	families  map[string]*family
	errs      []error
}

type family struct {
	name string // rendered, namespace-prefixed, _total-suffixed
	help string
	kind string
	// values sums samples by their serialized label set.
	values map[string]float64
	// labelSets remembers the labels behind each key.
	labelSets map[string]map[string]string
}

func newAggregator(namespace string) *aggregator {
	return &aggregator{namespace: namespace, families: map[string]*family{}}
}

// add folds one payload in. Owner samples are enriched with the owner's
// attributes (attrKeys is the label-name union; unmapped owners get
// "unknown") and never emitted per owner.
func (a *aggregator) add(svcName string, p *stats.Payload, attrs map[string]map[string]string, attrKeys []string) {
	for _, m := range p.Metrics {
		name := a.namespace + "_" + m.Name
		if m.Kind == stats.KindCounter {
			name += "_total"
		}
		f, ok := a.families[name]
		if !ok {
			f = &family{name: name, help: m.Help, kind: m.Kind,
				values: map[string]float64{}, labelSets: map[string]map[string]string{}}
			a.families[name] = f
			a.order = append(a.order, name)
		}
		if f.kind != m.Kind {
			a.errs = append(a.errs, fmt.Errorf("metric %s from %s: kind %s conflicts with %s", m.Name, svcName, m.Kind, f.kind))
			continue
		}
		if f.help == "" {
			f.help = m.Help
		}
		for _, s := range m.Samples {
			f.sum(s.Labels, s.Value)
		}
		for _, s := range m.OwnerSamples {
			f.sum(enrich(s.Labels, s.Owner, attrs, attrKeys), s.Value)
		}
	}
}

// enrich merges the owner's attribute labels into the sample labels.
func enrich(labels map[string]string, owner string, attrs map[string]map[string]string, attrKeys []string) map[string]string {
	if len(attrKeys) == 0 {
		return labels
	}
	merged := make(map[string]string, len(labels)+len(attrKeys))
	maps.Copy(merged, labels)
	ownerAttrs := attrs[owner]
	for _, k := range attrKeys {
		if v, ok := ownerAttrs[k]; ok && v != "" {
			merged[k] = v
		} else {
			merged[k] = "unknown"
		}
	}
	return merged
}

// sum accumulates a value under its label set.
func (f *family) sum(labels map[string]string, value float64) {
	key := labelKey(labels)
	f.values[key] += value
	if _, ok := f.labelSets[key]; !ok {
		f.labelSets[key] = labels
	}
}

// labelKey serializes a label set deterministically.
func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([][2]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, [2]string{k, labels[k]})
	}
	b, _ := json.Marshal(parts)
	return string(b)
}

// build renders every family into const metrics. Within a family the
// label-name set is the union over its samples; missing labels are "".
func (a *aggregator) build() ([]prometheus.Metric, []error) {
	var out []prometheus.Metric
	for _, name := range a.order {
		f := a.families[name]

		nameSet := map[string]struct{}{}
		for _, ls := range f.labelSets {
			for k := range ls {
				nameSet[k] = struct{}{}
			}
		}
		labelNames := make([]string, 0, len(nameSet))
		for k := range nameSet {
			labelNames = append(labelNames, k)
		}
		sort.Strings(labelNames)

		desc := prometheus.NewDesc(f.name, f.help, labelNames, nil)
		valueType := prometheus.GaugeValue
		if f.kind == stats.KindCounter {
			valueType = prometheus.CounterValue
		}

		keys := make([]string, 0, len(f.values))
		for k := range f.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			labels := f.labelSets[key]
			values := make([]string, len(labelNames))
			for i, ln := range labelNames {
				values[i] = labels[ln]
			}
			m, err := prometheus.NewConstMetric(desc, valueType, f.values[key], values...)
			if err != nil {
				a.errs = append(a.errs, fmt.Errorf("metric %s: %w", f.name, err))
				continue
			}
			out = append(out, m)
		}
	}
	return out, a.errs
}
