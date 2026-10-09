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
	"testing"
)

func validPayload() *Payload {
	return &Payload{
		Scope: ScopeShared,
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
		{Scope: "wat"},
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
	res, err := set.Invoke(context.Background(), Invocation, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(res)
	p, err := FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Scope != ScopeShared || len(p.Metrics) != 2 {
		t.Errorf("invoked payload = %+v", p)
	}
}
