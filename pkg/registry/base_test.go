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

package registry

import (
	"sync"
	"testing"
	"time"
)

// recordingDriver is a test Driver that records propagated writes and lets a
// test push remote events through the watch channel.
type recordingDriver struct {
	mu      sync.Mutex
	added   []string
	removed []string
	ch      chan Event
}

func newRecordingDriver() *recordingDriver {
	return &recordingDriver{ch: make(chan Event, 16)}
}

func (d *recordingDriver) Add(service string, n Node) error {
	d.mu.Lock()
	d.added = append(d.added, service+"/"+n.ID())
	d.mu.Unlock()
	return nil
}

func (d *recordingDriver) Remove(service, nodeID string) error {
	d.mu.Lock()
	d.removed = append(d.removed, service+"/"+nodeID)
	d.mu.Unlock()
	return nil
}

func (d *recordingDriver) Watch() (<-chan Event, error) { return d.ch, nil }
func (d *recordingDriver) Close()                       {}

func meta(state, seen string) map[string]string {
	return map[string]string{MetaState: state, MetaLastSeen: seen}
}

func TestBaseAddWritesCacheThenDriver(t *testing.T) {
	d := newRecordingDriver()
	b := NewBase(d, Thresholds{})
	defer b.Close()

	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "10.0.0.1:1", nil)}))

	// resolvable from the local cache immediately
	if _, err := b.GetService("gateway"); err != nil {
		t.Fatalf("expected local resolve: %v", err)
	}
	// and propagated to the driver
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.added) != 1 || d.added[0] != "gateway/n1" {
		t.Fatalf("expected driver.Add(gateway/n1), got %v", d.added)
	}
}

func TestBaseWatchAppliesRemoteEvents(t *testing.T) {
	d := newRecordingDriver()
	b := NewBase(d, Thresholds{})
	defer b.Close()

	d.ch <- Event{Type: EventAdd, Service: "userprovider", Node: NewNode("u1", "10.0.0.2:1", nil)}

	deadline := time.After(2 * time.Second)
	for {
		if _, err := b.GetService("userprovider"); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("remote add was not applied to the cache")
		case <-time.After(10 * time.Millisecond):
		}
	}

	d.ch <- Event{Type: EventRemove, Service: "userprovider", Node: NewNode("u1", "", nil)}
	deadline = time.After(2 * time.Second)
	for {
		if _, err := b.GetService("userprovider"); err != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("remote remove was not applied to the cache")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func rfc(now time.Time) string { return now.UTC().Format(time.RFC3339) }

func livenessThresholds() Thresholds {
	return Thresholds{DegradedAfter: 15 * time.Second, OfflineAfter: 30 * time.Second, ReapAfter: 5 * time.Minute}
}

func TestLivenessDegradesThenOffline(t *testing.T) {
	now := time.Now()

	b := NewBase(newRecordingDriver(), livenessThresholds())
	defer b.Close()
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateReady, rfc(now.Add(-20*time.Second))))}))
	b.sweep(now)
	if got := stateOf(t, b); got != StateDegraded {
		t.Fatalf("expected degraded after 20s, got %q", got)
	}

	b2 := NewBase(newRecordingDriver(), livenessThresholds())
	defer b2.Close()
	_ = b2.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateReady, rfc(now.Add(-40*time.Second))))}))
	b2.sweep(now)
	if got := stateOf(t, b2); got != StateOffline {
		t.Fatalf("expected offline after 40s, got %q", got)
	}
}

func TestLivenessRecoversOnBeat(t *testing.T) {
	now := time.Now()
	b := NewBase(newRecordingDriver(), livenessThresholds())
	defer b.Close()
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateOffline, rfc(now.Add(-40*time.Second))))}))
	b.sweep(now)
	if got := stateOf(t, b); got != StateOffline {
		t.Fatalf("expected offline, got %q", got)
	}
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateReady, rfc(now)))}))
	b.sweep(now)
	if got := stateOf(t, b); got != StateReady {
		t.Fatalf("expected recovery to ready, got %q", got)
	}
}

func TestLivenessReaps(t *testing.T) {
	now := time.Now()
	b := NewBase(newRecordingDriver(), livenessThresholds())
	defer b.Close()
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateOffline, rfc(now.Add(-10*time.Minute))))}))
	b.sweep(now)
	if _, err := b.GetService("gateway"); err == nil {
		t.Fatal("expected node to be reaped")
	}
}

func TestLivenessLeavesDraining(t *testing.T) {
	now := time.Now()
	b := NewBase(newRecordingDriver(), livenessThresholds())
	defer b.Close()
	// Quiet past offline but before reap: draining must not be auto-transitioned.
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateDraining, rfc(now.Add(-1*time.Minute))))}))
	b.sweep(now)
	if got := stateOf(t, b); got != StateDraining {
		t.Fatalf("draining must never be auto-derived, got %q", got)
	}
}

func TestLivenessReapsDeadDraining(t *testing.T) {
	now := time.Now()
	b := NewBase(newRecordingDriver(), livenessThresholds())
	defer b.Close()
	// A drained node long past the reap window is removed, not left as a ghost.
	_ = b.Add(NewService("gateway", []Node{NewNode("n1", "a:1", meta(StateDraining, rfc(now.Add(-1*time.Hour))))}))
	b.sweep(now)
	if _, err := b.GetService("gateway"); err == nil {
		t.Fatalf("dead draining node should have been reaped")
	}
}

func stateOf(t *testing.T, b *BaseRegistry) string {
	t.Helper()
	svc, err := b.GetService("gateway")
	if err != nil {
		return ""
	}
	return svc.Nodes()[0].Metadata()[MetaState]
}
