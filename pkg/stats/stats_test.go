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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cs3org/reva/v3/pkg/invoke"
)

func validPayload() *Payload {
	return &Payload{
		Metrics: []Metric{
			{Name: "shares", Kind: KindGauge, Samples: []Sample{
				{Labels: map[string]string{"status": "active"}, Value: 3},
			}},
			{Name: "projects", Kind: KindGauge, OwnerSamples: []OwnerSample{
				{Owner: "alice", Value: 2},
			}},
		},
	}
}

func TestValidate(t *testing.T) {
	if err := validPayload().Validate(); err != nil {
		t.Fatal(err)
	}

	bad := []*Payload{
		{Metrics: []Metric{{Name: "Bad-Name", Kind: KindGauge}}},
		{Metrics: []Metric{{Name: "ok", Kind: "histogram"}}},
		{Metrics: []Metric{{Name: "ok", Kind: KindGauge,
			Samples: []Sample{{Labels: map[string]string{"Bad-Label": "x"}}}}}},
		{Metrics: []Metric{{Name: "ok", Kind: KindGauge,
			OwnerSamples: []OwnerSample{{Owner: "", Value: 1}}}}},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("payload %d: expected validation error", i)
		}
	}
}

func TestResultRoundTrip(t *testing.T) {
	res, err := validPayload().Result()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	p, err := FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Metrics) != 2 || p.Metrics[0].Name != "shares" ||
		p.Metrics[0].Samples[0].Value != 3 ||
		p.Metrics[1].OwnerSamples[0].Owner != "alice" {
		t.Errorf("round trip mangled the payload: %+v", p)
	}
}

type fakeReporter struct{ p *Payload }

func (f fakeReporter) Stats(context.Context) (*Payload, error) { return f.p, nil }

func TestNewInvokeSet(t *testing.T) {
	// non-reporter: empty set
	set := NewInvokeSet(struct{}{})
	if got := len(set.Invocations()); got != 0 {
		t.Fatalf("non-reporter registered %d invocations", got)
	}

	// reporter: stats invocation wired through
	set = NewInvokeSet(fakeReporter{p: validPayload()})
	specs := set.Invocations()
	if len(specs) != 1 || specs[0].Name != Invocation {
		t.Fatalf("invocations = %+v", specs)
	}
	var metrics []Metric
	err := set.InvokeStream(context.Background(), Invocation, nil, func(res invoke.Result) error {
		data, _ := json.Marshal(res)
		p, err := FromJSON(data)
		if err != nil {
			return err
		}
		metrics = append(metrics, p.Metrics...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 2 {
		t.Errorf("invoked metrics = %+v", metrics)
	}
}

func TestChunks(t *testing.T) {
	var owners []OwnerSample
	for i := range 25 {
		owners = append(owners, OwnerSample{Owner: fmt.Sprintf("u%d", i), Value: 1})
	}
	p := &Payload{Metrics: []Metric{
		{Name: "shares", Kind: KindGauge, Samples: []Sample{{Value: 3}}},
		{Name: "projects", Kind: KindGauge, OwnerSamples: owners},
	}}

	chunks := p.chunks(10)
	if len(chunks) != 4 {
		t.Fatalf("chunks = %d, want 4 (1 for shares, 3 for projects)", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		for _, m := range c.Metrics {
			if n := len(m.Samples) + len(m.OwnerSamples); n > 10 {
				t.Errorf("chunk of %s has %d samples, want at most 10", m.Name, n)
			}
			if m.Name == "projects" {
				total += len(m.OwnerSamples)
			}
		}
	}
	if total != 25 {
		t.Errorf("projects owner samples = %d, want 25", total)
	}
}
