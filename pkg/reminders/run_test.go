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

package reminders

import (
	"context"
	"errors"
	"iter"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeKind struct {
	items []Item
	err   error
	// seen collects what Notice got, by recipient.
	seen map[string][]string
}

func (k *fakeKind) Event() string { return "fake" }

func (k *fakeKind) Scan(context.Context) iter.Seq2[Item, error] {
	return func(yield func(Item, error) bool) {
		for _, it := range k.items {
			if !yield(it, nil) {
				return
			}
		}
		if k.err != nil {
			yield(Item{}, k.err)
		}
	}
}

func (k *fakeKind) Notice(_ context.Context, to Recipient, items iter.Seq[Item]) (map[string]any, error) {
	var subjects []string
	for it := range items {
		subjects = append(subjects, it.Subject)
	}
	k.seen[to.Name] = subjects
	return map[string]any{"subjects": subjects}, nil
}

type harness struct {
	kind      *fakeKind
	runner    *Runner
	now       time.Time
	published map[string][]string
	fail      map[string]bool
}

func newHarness(t *testing.T) *harness {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "reminders.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	l := &Ledger{DB: db}
	if err := l.Migrate(); err != nil {
		t.Fatal(err)
	}
	h := &harness{kind: &fakeKind{}, now: time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC), fail: map[string]bool{}}
	h.runner = &Runner{
		Name:   "fake",
		Kind:   h.kind,
		Ledger: l,
		Now:    func() time.Time { return h.now },
		Publish: func(_ context.Context, _ string, to Recipient, data map[string]any) error {
			if h.fail[to.Name] {
				return errors.New("refused")
			}
			h.published[to.Name] = data["subjects"].([]string)
			return nil
		},
	}
	return h
}

// run scans items one day after the previous run and returns what was
// published, by recipient.
func (h *harness) run(t *testing.T, items ...Item) (map[string][]string, error) {
	t.Helper()
	h.now = h.now.Add(24 * time.Hour)
	h.kind.items = items
	h.kind.seen = map[string][]string{}
	h.published = map[string][]string{}
	_, err := h.runner.Run(context.Background())
	return h.published, err
}

func item(subject string, level int, to string) Item {
	return Item{Subject: subject, Level: level, Recipient: Recipient{Name: to}}
}

func expect(t *testing.T, got map[string][]string, want map[string][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("published %v, want %v", got, want)
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Fatalf("published %v, want %v", got, want)
		}
	}
}

func TestRunTellsOncePerLevel(t *testing.T) {
	h := newHarness(t)
	got, err := h.run(t, item("a", 1, "alice"), item("b", 0, "alice"), item("c", 1, "bob"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, got, map[string][]string{"alice": {"a"}, "bob": {"c"}})

	got, _ = h.run(t, item("a", 1, "alice"), item("b", 0, "alice"), item("c", 1, "bob"))
	expect(t, got, nil)

	got, _ = h.run(t, item("a", 2, "alice"), item("b", 1, "alice"), item("c", 1, "bob"))
	expect(t, got, map[string][]string{"alice": {"a", "b"}})

	got, _ = h.run(t, item("a", 1, "alice"), item("b", 1, "alice"), item("c", 1, "bob"))
	expect(t, got, nil)
}

func TestRunOrdersItemsByLevelThenSubject(t *testing.T) {
	h := newHarness(t)
	got, _ := h.run(t, item("b", 1, "alice"), item("c", 2, "alice"), item("a", 2, "alice"))
	expect(t, got, map[string][]string{"alice": {"a", "c", "b"}})
}

func TestRunClosesOnlyAfterACompleteScan(t *testing.T) {
	h := newHarness(t)
	h.run(t, item("a", 1, "alice"))

	h.kind.err = errors.New("source down")
	if _, err := h.run(t); err == nil {
		t.Fatal("a partial scan must fail the run")
	}
	h.kind.err = nil
	got, _ := h.run(t, item("a", 1, "alice"))
	expect(t, got, nil)

	h.run(t)
	got, _ = h.run(t, item("a", 1, "alice"))
	expect(t, got, map[string][]string{"alice": {"a"}})
}

func TestRunRetriesOnlyWhatFailed(t *testing.T) {
	h := newHarness(t)
	h.fail["bob"] = true
	got, err := h.run(t, item("a", 1, "alice"), item("c", 1, "bob"))
	if err == nil {
		t.Fatal("a failed notification must fail the run")
	}
	expect(t, got, map[string][]string{"alice": {"a"}})

	h.fail["bob"] = false
	got, _ = h.run(t, item("a", 1, "alice"), item("c", 1, "bob"))
	expect(t, got, map[string][]string{"bob": {"c"}})
}

func TestRunLeavesItemsClaimedByAnotherRun(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	other := []Item{item("a", 1, "alice")}
	if err := h.runner.Ledger.Observe(ctx, "fake", other, h.now); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Ledger.Claim(ctx, "fake", "other", other, h.now.Add(23*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := h.run(t, item("a", 1, "alice"))
	expect(t, got, nil)

	got, _ = h.run(t, item("a", 1, "alice"))
	expect(t, got, map[string][]string{"alice": {"a"}})
}

func TestCheckWritesNothing(t *testing.T) {
	h := newHarness(t)
	h.run(t, item("a", 1, "alice"))
	h.kind.items = []Item{item("a", 2, "alice"), item("b", 1, "bob"), item("c", 0, "bob")}
	report, due, err := h.runner.Check(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Due != 2 || report.Recipients != 2 || len(due) != 2 || due[0].LevelSent != 1 {
		t.Fatalf("report %+v, due %+v", report, due)
	}
	episodes, err := h.runner.Ledger.Episodes(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].LevelSent != 1 {
		t.Fatalf("check changed the ledger: %+v", episodes)
	}
}

func TestHistorySinceInAnotherZone(t *testing.T) {
	h := newHarness(t)
	h.run(t, item("a", 1, "alice"))
	since := h.now.Add(-time.Hour).In(time.FixedZone("CEST", 2*60*60))
	sent, err := h.runner.Ledger.History(context.Background(), Filter{Since: since})
	if err != nil || len(sent) != 1 {
		t.Fatalf("sent %+v, %v", sent, err)
	}
}
