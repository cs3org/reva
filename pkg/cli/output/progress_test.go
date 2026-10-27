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

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func clockFrom(start time.Time, step *time.Duration) func() time.Time {
	return func() time.Time { return start.Add(*step) }
}

var meterEpoch = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

func TestMeterNeverExceedsTheWidth(t *testing.T) {
	for _, width := range []int{200, 120, 80, 60, 40, 30, 20, 10, 5, 1} {
		for _, done := range []int64{0, 1, 5 << 20, 100 << 20, 199 << 20, 200 << 20} {
			m := &Meter{label: "histograms.root", total: 200 << 20, done: done}
			line := m.render(width, 4*time.Second)
			if got := len([]rune(line)); got > width {
				t.Errorf("width %d, done %d: line is %d wide:\n%q", width, done, got, line)
			}
		}
	}
}

func TestMeterLongLabelDoesNotCrowdOutTheNumbers(t *testing.T) {
	m := &Meter{
		label: "data/run3/2026/calibration/histograms-with-a-very-long-name.root",
		total: 200 << 20,
		done:  100 << 20,
	}
	line := m.render(80, time.Second)

	if len([]rune(line)) > 80 {
		t.Fatalf("the line is %d wide:\n%q", len([]rune(line)), line)
	}
	if !strings.Contains(line, "100.0M/200.0M") {
		t.Errorf("the counts were crowded out:\n%q", line)
	}
	if !strings.Contains(line, " 50%") {
		t.Errorf("the percentage was crowded out:\n%q", line)
	}
	if !strings.Contains(line, "histograms-with-a-very-long") && !strings.Contains(line, ".root") {
		t.Errorf("the label lost its distinguishing end:\n%q", line)
	}
}

func TestMeterWithoutATotalShowsNoPercentage(t *testing.T) {
	m := &Meter{label: "stdin", total: -1, done: 5 << 20}
	line := m.render(80, 2*time.Second)

	if strings.Contains(line, "%") {
		t.Errorf("there is no total, so there can be no percentage:\n%q", line)
	}
	if strings.Contains(line, meterFilled) || strings.Contains(line, meterEmpty) {
		t.Errorf("there is no total, so there can be no bar:\n%q", line)
	}
	if !strings.Contains(line, "5.0M") {
		t.Errorf("the byte count is the useful half and is missing:\n%q", line)
	}
	if !strings.Contains(line, "/s") {
		t.Errorf("the rate is missing:\n%q", line)
	}
}

func TestMeterPercentageAndBarTrackProgress(t *testing.T) {
	cases := []struct {
		done    int64
		wantPct string
	}{
		{0, "  0%"},
		{50, " 50%"},
		{99, " 99%"},
		{100, "100%"},
		// A stale size on the server can give more than the total.
		{143, "100%"},
	}
	for _, c := range cases {
		m := &Meter{label: "f", total: 100, done: c.done}
		line := m.render(80, time.Second)
		if !strings.Contains(line, c.wantPct) {
			t.Errorf("done %d: want %q in:\n%q", c.done, c.wantPct, line)
		}
	}
}

func TestMeterBarIsNeverOverfilled(t *testing.T) {
	m := &Meter{label: "f", total: 100, done: 500}
	line := m.render(80, time.Second)
	if len([]rune(line)) > 80 {
		t.Errorf("the overfilled bar spilled: %d wide\n%q", len([]rune(line)), line)
	}
	if strings.Contains(line, meterEmpty+meterFilled) {
		t.Errorf("the bar is not contiguous:\n%q", line)
	}
}

func TestMeterRateAndETA(t *testing.T) {
	m := &Meter{label: "f", total: 200 << 20, done: 100 << 20}
	line := m.render(100, 10*time.Second)

	if !strings.Contains(line, "10.0M/s") {
		t.Errorf("the rate is wrong:\n%q", line)
	}
	if !strings.Contains(line, "eta 10s") {
		t.Errorf("the eta is wrong:\n%q", line)
	}
}

func TestMeterNoETAWhenItWouldBeMeaningless(t *testing.T) {
	fresh := &Meter{label: "f", total: 100, done: 0}
	if line := fresh.render(80, 0); strings.Contains(line, "eta") {
		t.Errorf("no bytes and no time should mean no eta:\n%q", line)
	}
	finished := &Meter{label: "f", total: 100, done: 100}
	if line := finished.render(80, time.Second); strings.Contains(line, "eta") {
		t.Errorf("a finished transfer needs no eta:\n%q", line)
	}
}

func TestMeterRedrawIsThrottled(t *testing.T) {
	var buf bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&buf, "f", 1000, MeterWidth(80), MeterClock(clockFrom(meterEpoch, &step)))

	m.Add(100)
	first := strings.Count(buf.String(), "\r")
	if first != 1 {
		t.Fatalf("the first update should draw once, got %d", first)
	}

	for range 10 {
		step += 5 * time.Millisecond
		m.Add(10)
	}
	if got := strings.Count(buf.String(), "\r"); got != 1 {
		t.Errorf("updates inside the interval should not redraw, got %d draws", got)
	}

	step += meterRedraw
	m.Add(10)
	if got := strings.Count(buf.String(), "\r"); got != 2 {
		t.Errorf("an update past the interval should redraw, got %d draws", got)
	}
}

func TestMeterStopErasesItsLine(t *testing.T) {
	var buf bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&buf, "f", 1000, MeterWidth(80), MeterClock(clockFrom(meterEpoch, &step)))

	m.Add(500)
	drawn := buf.String()
	if !strings.Contains(drawn, "50%") {
		t.Fatalf("nothing was drawn to erase:\n%q", drawn)
	}

	buf.Reset()
	m.Stop()
	erased := buf.String()
	if !strings.HasPrefix(erased, "\r") || !strings.HasSuffix(erased, "\r") {
		t.Errorf("the erase should return to the start of the line both ways:\n%q", erased)
	}
	if strings.TrimSpace(strings.Trim(erased, "\r")) != "" {
		t.Errorf("the erase should write only blanks:\n%q", erased)
	}

	buf.Reset()
	m.Stop()
	if buf.Len() != 0 {
		t.Errorf("a second Stop wrote %q", buf.String())
	}
	m.Add(100)
	if buf.Len() != 0 {
		t.Errorf("an update after Stop wrote %q", buf.String())
	}
}

func TestMeterWithABarFillsTheWidthExactly(t *testing.T) {
	for _, done := range []int64{1, 40, 99, 100} {
		m := &Meter{label: "f", total: 100, done: done}
		line := m.render(70, time.Second)
		if got := len([]rune(line)); got != 70 {
			t.Errorf("done %d: line is %d wide, want exactly 70:\n%q", done, got, line)
		}
	}
}

func TestMeterShrinkingLineIsPaddedOver(t *testing.T) {
	var buf bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&buf, "a-rather-long-name.bin", -1, MeterWidth(80),
		MeterClock(clockFrom(meterEpoch, &step)))

	step = time.Second
	m.Add(1000)
	longLine := strings.TrimPrefix(buf.String(), "\r")

	m.SetLabel("f")
	step += meterRedraw
	buf.Reset()
	m.Add(0)

	shortDraw := buf.String()
	shortLine := strings.TrimPrefix(shortDraw, "\r")
	if len([]rune(shortLine)) <= len([]rune(longLine)) && !strings.HasSuffix(shortDraw, " ") {
		t.Errorf("the shorter line was not padded over the longer one:\nlong:  %q\nshort: %q",
			longLine, shortDraw)
	}
	if got := len([]rune(shortDraw)) - 1; got < len([]rune(longLine)) {
		t.Errorf("the redraw covers %d columns, the previous line used %d:\n%q",
			got, len([]rune(longLine)), shortDraw)
	}
}

func TestMeterWriterCountsWhatPasses(t *testing.T) {
	var bar, sink bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&bar, "f", 11, MeterWidth(80), MeterClock(clockFrom(meterEpoch, &step)))

	w := m.Writer(&sink)
	if _, err := w.Write([]byte("hello ")); err != nil {
		t.Fatal(err)
	}
	step += meterRedraw
	if _, err := w.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}

	if sink.String() != "hello world" {
		t.Errorf("the destination got %q", sink.String())
	}
	if !strings.Contains(bar.String(), "100%") {
		t.Errorf("the meter did not reach 100%%:\n%q", bar.String())
	}
}

func TestMeterWriterPropagatesErrors(t *testing.T) {
	var bar bytes.Buffer
	m := NewMeter(&bar, "f", 10, MeterWidth(80))

	w := m.Writer(failingWriter{})
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("the write error should reach the caller")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestMeterSetTakesAnAbsolutePosition(t *testing.T) {
	var buf bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&buf, "f", -1, MeterWidth(80), MeterClock(clockFrom(meterEpoch, &step)))

	m.Set(25, 100)
	if !strings.Contains(buf.String(), " 25%") {
		t.Errorf("Set should have supplied both position and total:\n%q", buf.String())
	}

	step += meterRedraw
	buf.Reset()
	m.Set(50, 0)
	if !strings.Contains(buf.String(), " 50%") {
		t.Errorf("a zero total should be ignored:\n%q", buf.String())
	}
}

func TestMeterSetLabel(t *testing.T) {
	var buf bytes.Buffer
	step := time.Duration(0)
	m := NewMeter(&buf, "first", 100, MeterWidth(80), MeterClock(clockFrom(meterEpoch, &step)))

	m.SetLabel("second")
	step += meterRedraw
	m.Add(10)
	if !strings.Contains(buf.String(), "second") {
		t.Errorf("the new label is missing:\n%q", buf.String())
	}
}

func TestTruncateLabel(t *testing.T) {
	if got := truncateLabel("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	got := truncateLabel("averylongfilenamethatkeepsgoing", 10)
	if len([]rune(got)) != 10 {
		t.Errorf("truncated to %d runes: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "going") {
		t.Errorf("the end should survive: %q", got)
	}
	if !strings.HasPrefix(got, "…") {
		t.Errorf("the cut should be marked: %q", got)
	}
}

func TestRatePerSecond(t *testing.T) {
	if got := ratePerSecond(1000, 2*time.Second); got != 500 {
		t.Errorf("got %d, want 500", got)
	}
	if got := ratePerSecond(1000, 0); got != 0 {
		t.Errorf("no elapsed time should mean no rate, got %d", got)
	}
	if got := ratePerSecond(0, time.Second); got != 0 {
		t.Errorf("no bytes should mean no rate, got %d", got)
	}
}

func TestEtaOf(t *testing.T) {
	if s, ok := etaOf(50, 100, 25); !ok || s != "2s" {
		t.Errorf("got %q, %v", s, ok)
	}
	if _, ok := etaOf(50, 100, 0); ok {
		t.Error("no rate should mean no eta")
	}
	if _, ok := etaOf(100, 100, 25); ok {
		t.Error("a finished transfer should have no eta")
	}
}

func TestTrimToWidth(t *testing.T) {
	if got := trimToWidth("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := trimToWidth("hello", 3); got != "hel" {
		t.Errorf("got %q", got)
	}
	if got := trimToWidth("hello", 0); got != "" {
		t.Errorf("got %q", got)
	}
	if got := trimToWidth("héllo, world", 4); len([]rune(got)) != 4 {
		t.Errorf("got %q (%d runes)", got, len([]rune(got)))
	}
}
