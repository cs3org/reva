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

// Package stats defines the self-describing statistics payload a service
// can expose through its `stats` invocation, and the Reporter capability
// a driver implements to produce it.
//
// A service whose manager implements Reporter registers the "stats"
// invocation; the stats prometheus collector (pkg/prom/stats) discovers
// those services through the registry and turns their payloads into
// metrics. The payload fully describes its metrics (names, kinds, labels),
// so new services need no collector changes.
//
// Counters derived from creation-time row counts are monotonic as long as
// rows are soft-deleted; a hard-delete cleanup shows up in Prometheus as a
// counter reset.
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/cs3org/reva/v3/pkg/invoke"
)

// Invocation is the conventional invocation name under which services
// expose their statistics.
const Invocation = "stats"

// Scope declares the state topology of a service's statistics.
const (
	// ScopeShared means every instance reports the same state (e.g. a
	// shared database): the collector queries one live instance.
	ScopeShared = "shared"
	// ScopeInstance means each instance holds its own state: the
	// collector fans out and sums.
	ScopeInstance = "instance"
)

// Metric kinds.
const (
	KindGauge   = "gauge"
	KindCounter = "counter"
)

// Reporter is the capability a manager (driver) implements to expose
// statistics. Services detect it by type assertion and register the
// "stats" invocation only when it holds.
type Reporter interface {
	Stats(ctx context.Context) (*Payload, error)
}

// Payload is the return value of a "stats" invocation.
type Payload struct {
	// Scope is ScopeShared (default when empty) or ScopeInstance.
	Scope string `json:"scope,omitempty"`
	// Metrics are the self-described metric families.
	Metrics []Metric `json:"metrics"`
}

// Metric is one metric family. Name carries the subject only — the
// collector prefixes the configured namespace and appends the
// Prometheus-conventional `_total` suffix to counters.
type Metric struct {
	Name string `json:"name"`
	Help string `json:"help,omitempty"`
	// Kind is KindGauge or KindCounter.
	Kind string `json:"kind"`
	// Samples are plain samples.
	Samples []Sample `json:"samples,omitempty"`
	// OwnerSamples are per-owner aggregates. The collector folds them by
	// the owner attributes available at the site (or into plain totals);
	// they are never exposed as per-owner series.
	OwnerSamples []OwnerSample `json:"owner_samples,omitempty"`
}

// Sample is one value with its label set.
type Sample struct {
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// OwnerSample is one per-owner value with an optional label set.
type OwnerSample struct {
	Owner  string            `json:"owner"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks the payload is well formed: legal metric and label
// names, known kinds and scope.
func (p *Payload) Validate() error {
	switch p.Scope {
	case "", ScopeShared, ScopeInstance:
	default:
		return fmt.Errorf("stats: unknown scope %q", p.Scope)
	}
	for _, m := range p.Metrics {
		if !nameRe.MatchString(m.Name) {
			return fmt.Errorf("stats: metric name %q does not conform to [a-z][a-z0-9_]*", m.Name)
		}
		if m.Kind != KindGauge && m.Kind != KindCounter {
			return fmt.Errorf("stats: metric %s: unknown kind %q", m.Name, m.Kind)
		}
		for _, s := range m.Samples {
			if err := validateLabels(m.Name, s.Labels); err != nil {
				return err
			}
		}
		for _, s := range m.OwnerSamples {
			if s.Owner == "" {
				return fmt.Errorf("stats: metric %s: owner sample without owner", m.Name)
			}
			if err := validateLabels(m.Name, s.Labels); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateLabels(metric string, labels map[string]string) error {
	for l := range labels {
		if !nameRe.MatchString(l) {
			return fmt.Errorf("stats: metric %s: label name %q does not conform to [a-z][a-z0-9_]*", metric, l)
		}
	}
	return nil
}

// Result validates the payload and converts it to an invoke.Result (the
// JSON-friendly map an invocation returns).
func (p *Payload) Result() (invoke.Result, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var out invoke.Result
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FromJSON parses a payload from an invocation's JSON result and
// validates it.
func FromJSON(data []byte) (*Payload, error) {
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("stats: parsing payload: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// NewInvokeSet returns an invocation set exposing the "stats" invocation
// when the manager implements Reporter, and an empty set otherwise. A
// service embeds the returned set to become invokable, so the capability
// of its driver decides whether it advertises statistics.
func NewInvokeSet(manager any) *invoke.Set {
	set := invoke.NewSet()
	r, ok := manager.(Reporter)
	if !ok {
		return set
	}
	set.Add(Invocation, "Service statistics as a self-describing metrics payload.").
		Handle(func(ctx context.Context, _ invoke.Args) (invoke.Result, error) {
			p, err := r.Stats(ctx)
			if err != nil {
				return nil, err
			}
			return p.Result()
		})
	return set
}
