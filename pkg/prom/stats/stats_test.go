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
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/stats"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// fakeFleet serves canned payloads per endpoint node id. Like the Admin
// API, it answers a service with the payloads of all its listed instances.
type fakeFleet struct {
	targets  map[string][]client.Endpoint
	payloads map[string]*stats.Payload
}

func (f fakeFleet) StatsServices(context.Context) ([]string, error) {
	return slices.Sorted(maps.Keys(f.targets)), nil
}
func (f fakeFleet) InvokeStats(_ context.Context, svc string) ([]*stats.Payload, error) {
	var out []*stats.Payload
	for _, ep := range f.targets[svc] {
		p, ok := f.payloads[ep.Node]
		if !ok {
			return nil, fmt.Errorf("no payload for %s", ep.Node)
		}
		out = append(out, p)
	}
	return out, nil
}

// gather renders the collector's metrics as text lines "name{labels} value".
func gather(t *testing.T, c *collector) map[string]float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 100)
	c.Collect(ch)
	close(ch)
	out := map[string]float64{}
	for m := range ch {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			t.Fatal(err)
		}
		var labels []string
		for _, l := range d.Label {
			labels = append(labels, l.GetName()+"="+l.GetValue())
		}
		key := descName(m.Desc().String()) + "{" + strings.Join(labels, ",") + "}"
		switch {
		case d.Gauge != nil:
			out[key] = d.Gauge.GetValue()
		case d.Counter != nil:
			out[key] = d.Counter.GetValue()
		}
	}
	return out
}

// descName extracts the fqName from a Desc's String() form.
func descName(s string) string {
	const marker = `fqName: "`
	i := strings.Index(s, marker)
	if i < 0 {
		return s
	}
	s = s[i+len(marker):]
	return s[:strings.Index(s, `"`)]
}

func shared(metrics ...stats.Metric) *stats.Payload {
	return &stats.Payload{Metrics: metrics}
}

func TestRefreshMergesFamiliesAcrossServices(t *testing.T) {
	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"usershareprovider":   {{Node: "n1", Addr: "a1", Target: "n1"}},
			"publicshareprovider": {{Node: "n2", Addr: "a2", Target: "n2"}},
		},
		payloads: map[string]*stats.Payload{
			"n1": shared(stats.Metric{Name: "shares", Kind: stats.KindGauge, Samples: []stats.Sample{
				{Labels: map[string]string{"kind": "share", "status": "active"}, Value: 5},
			}}),
			"n2": shared(stats.Metric{Name: "shares", Kind: stats.KindGauge, Samples: []stats.Sample{
				{Labels: map[string]string{"kind": "link", "status": "active"}, Value: 2},
			}}),
		},
	}
	c := &collector{namespace: "reva", attrFile: "/nonexistent", fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_shares{kind=share,status=active}"] != 5 {
		t.Errorf("share sample missing: %v", got)
	}
	if got["reva_shares{kind=link,status=active}"] != 2 {
		t.Errorf("link sample missing: %v", got)
	}
}

func TestInstanceScopeSums(t *testing.T) {
	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"gateway": {{Node: "n1", Addr: "a1"}, {Node: "n2", Addr: "a2"}},
		},
		payloads: map[string]*stats.Payload{
			"n1": {Metrics: []stats.Metric{
				{Name: "sessions", Kind: stats.KindGauge, Samples: []stats.Sample{{Value: 3}}}}},
			"n2": {Metrics: []stats.Metric{
				{Name: "sessions", Kind: stats.KindGauge, Samples: []stats.Sample{{Value: 4}}}}},
		},
	}
	c := &collector{namespace: "reva", attrFile: "/nonexistent", fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_sessions{}"] != 7 {
		t.Errorf("instance-scope sum = %v, want 7: %v", got["reva_sessions{}"], got)
	}
}

func TestOwnerAttributeEnrichment(t *testing.T) {
	attrFile := filepath.Join(t.TempDir(), "owners.json")
	os.WriteFile(attrFile, []byte(`{
		"alice": {"department": "IT", "experiment": "ATLAS"},
		"bob":   {"department": "EP"}
	}`), 0o644)

	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"projects": {{Node: "n1", Addr: "a1"}},
		},
		payloads: map[string]*stats.Payload{
			"n1": shared(stats.Metric{Name: "projects", Kind: stats.KindGauge, OwnerSamples: []stats.OwnerSample{
				{Owner: "alice", Value: 2},
				{Owner: "bob", Value: 1},
				{Owner: "carol", Value: 4}, // unmapped
			}}),
		},
	}
	c := &collector{namespace: "reva", attrFile: attrFile, fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_projects{department=IT,experiment=ATLAS}"] != 2 {
		t.Errorf("alice bucket: %v", got)
	}
	if got["reva_projects{department=EP,experiment=unknown}"] != 1 {
		t.Errorf("bob bucket (partial attrs): %v", got)
	}
	if got["reva_projects{department=unknown,experiment=unknown}"] != 4 {
		t.Errorf("carol bucket (unmapped): %v", got)
	}
}

func TestOwnerAttributesKeyAndFilter(t *testing.T) {
	attrFile := filepath.Join(t.TempDir(), "affiliation.json")
	os.WriteFile(attrFile, []byte(`{
		"generated": "2026-10-23T00:30:00Z",
		"entries": {
			"alice": {"department": "IT", "employee_id": "1"},
			"bob":   {"department": "EP", "employee_id": "2"}
		}
	}`), 0o644)

	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"projects": {{Node: "n1", Addr: "a1"}},
		},
		payloads: map[string]*stats.Payload{
			"n1": shared(stats.Metric{Name: "projects", Kind: stats.KindGauge, OwnerSamples: []stats.OwnerSample{
				{Owner: "alice", Value: 2},
				{Owner: "bob", Value: 1},
			}}),
		},
	}
	c := &collector{namespace: "reva", attrFile: attrFile, attrKey: "entries", attrKeys: []string{"department"},
		fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_projects{department=IT}"] != 2 || got["reva_projects{department=EP}"] != 1 {
		t.Errorf("want one series per department, without employee_id: %v", got)
	}
}

func TestOwnerSamplesCollapseWithoutAttributes(t *testing.T) {
	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"projects": {{Node: "n1", Addr: "a1"}},
		},
		payloads: map[string]*stats.Payload{
			"n1": shared(stats.Metric{Name: "projects", Kind: stats.KindGauge, OwnerSamples: []stats.OwnerSample{
				{Owner: "alice", Value: 2},
				{Owner: "bob", Value: 1},
			}}),
		},
	}
	c := &collector{namespace: "reva", attrFile: "/nonexistent", fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_projects{}"] != 3 {
		t.Errorf("collapsed total = %v, want 3 (no per-owner series): %v", got["reva_projects{}"], got)
	}
	for k := range got {
		if strings.Contains(k, "alice") || strings.Contains(k, "bob") {
			t.Errorf("per-owner series leaked: %s", k)
		}
	}
}

func TestCounterSuffixAndSelfMetrics(t *testing.T) {
	f := fakeFleet{
		targets: map[string][]client.Endpoint{
			"usershareprovider": {{Node: "n1", Addr: "a1"}},
			"broken":            {{Node: "nX", Addr: "aX"}}, // no payload -> error
		},
		payloads: map[string]*stats.Payload{
			"n1": shared(stats.Metric{Name: "shares_created", Kind: stats.KindCounter, Samples: []stats.Sample{
				{Labels: map[string]string{"kind": "share"}, Value: 41},
			}}),
		},
	}
	c := &collector{namespace: "reva", attrFile: "/nonexistent", fleet: f, errors: map[string]float64{}}
	c.refreshOnce(context.Background())

	got := gather(t, c)
	if got["reva_shares_created_total{kind=share}"] != 41 {
		t.Errorf("counter not suffixed/valued: %v", got)
	}
	if got["reva_stats_refresh_errors_total{service=broken}"] != 1 {
		t.Errorf("refresh error not counted: %v", got)
	}
	found := false
	for k := range got {
		if strings.HasPrefix(k, "reva_stats_refresh_timestamp_seconds{service=usershareprovider}") {
			found = true
		}
	}
	if !found {
		t.Errorf("refresh timestamp missing: %v", got)
	}
}
