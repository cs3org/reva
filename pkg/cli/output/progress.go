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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Meter draws a transfer's progress on a single line, rewriting it in place.
//
// It is deliberately separate from the transfer engine's event stream, because
// the same bar has to be driven two different ways. A download through the engine
// reports its progress as events; a stream joined or handed over by the clipboard
// is a plain io.Copy, with nothing to report anything. Meter accepts both: Add
// for the first, Writer for the second.
//
// Everything about the rendering is a pure function of the meter's state and the
// terminal width, so the layout can be tested without a terminal: which matters,
// because the one thing a progress bar must never do is be wider than the window
// and wrap, turning one line into a screenful of debris.
type Meter struct {
	out io.Writer

	mu       sync.Mutex
	label    string
	total    int64 // -1 when the size is not known in advance
	done     int64
	start    time.Time
	lastDraw time.Time
	lastLen  int
	stopped  bool

	width int
	now   func() time.Time
}

const (
	// The bar is drawn with block characters rather than ASCII. Every terminal
	// this runs in is UTF-8, and the CLI already emits ANSI colour, so the bar may
	// as well look like the decade it was written in. They are constants so that
	// swapping them for "=" and "-" is a one-line change if that turns out wrong.
	meterFilled = "█"
	meterEmpty  = "░"

	// meterRedraw bounds how often the line is rewritten. A bar redrawn per read
	// on a fast link is pure waste, and on a slow terminal it is worse than waste.
	meterRedraw = 100 * time.Millisecond

	// meterMinBar is the narrowest bar worth drawing. Below it the bar is dropped
	// and the numbers keep the line, which is the useful half anyway.
	meterMinBar = 8

	// meterMaxLabel bounds the name so that a deep path cannot crowd out the
	// numbers entirely.
	meterMaxLabel = 28
)

// MeterOption configures a Meter.
type MeterOption func(*Meter)

// MeterWidth fixes the width the bar lays itself out in. Without it the meter
// asks the terminal on every draw, which is what makes it survive a resize.
func MeterWidth(n int) MeterOption { return func(m *Meter) { m.width = n } }

// MeterClock replaces the clock, so that rates and ETAs are testable.
func MeterClock(f func() time.Time) MeterOption { return func(m *Meter) { m.now = f } }

// NewMeter returns a Meter drawing to out. A total of -1 means the size is not
// known, which is the normal case for a stream: the bar becomes a byte count and
// a rate, with no percentage it would have to invent.
func NewMeter(out io.Writer, label string, total int64, opts ...MeterOption) *Meter {
	m := &Meter{out: out, label: label, total: total, now: time.Now}
	for _, o := range opts {
		o(m)
	}
	m.start = m.now()
	// Backdated so the first update draws immediately rather than after a tenth of
	// a second of apparently nothing happening.
	m.lastDraw = m.start.Add(-meterRedraw)
	return m
}

// SetLabel names what is being transferred now, for a paste that covers several
// items.
func (m *Meter) SetLabel(label string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.label = label
}

// Add records bytes transferred and redraws if it is time to.
func (m *Meter) Add(n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done += n
	m.draw(false)
}

// Set records an absolute position, for a caller whose progress arrives as a
// running total rather than as increments: the transfer engine's events do.
func (m *Meter) Set(done, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done = done
	if total > 0 {
		m.total = total
	}
	m.draw(false)
}

// Stop erases the line. The command's own summary follows it, so leaving a
// finished bar on screen would only say the same thing twice.
func (m *Meter) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true
	if m.lastLen > 0 {
		fmt.Fprintf(m.out, "\r%s\r", strings.Repeat(" ", m.lastLen))
		m.lastLen = 0
	}
}

// Writer returns dst wrapped so that everything written through it counts
// towards the meter. This is how a plain io.Copy gets a progress bar.
func (m *Meter) Writer(dst io.Writer) io.Writer { return &meterWriter{m: m, dst: dst} }

type meterWriter struct {
	m   *Meter
	dst io.Writer
}

func (w *meterWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if n > 0 {
		w.m.Add(int64(n))
	}
	return n, err
}

// draw rewrites the line, subject to the redraw interval. The caller holds the
// lock.
func (m *Meter) draw(force bool) {
	if m.stopped {
		return
	}
	now := m.now()
	if !force && now.Sub(m.lastDraw) < meterRedraw {
		return
	}
	m.lastDraw = now

	line := m.render(m.widthNow(), now.Sub(m.start))

	// Padded to the previous length rather than cleared with an escape sequence:
	// the result is the same, and it keeps the output something a test can assert
	// on character for character.
	pad := ""
	if n := m.lastLen - len([]rune(line)); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	fmt.Fprintf(m.out, "\r%s%s", line, pad)
	m.lastLen = len([]rune(line))
}

func (m *Meter) widthNow() int {
	if m.width > 0 {
		return m.width
	}
	return TerminalWidth(stderrFile())
}

// render lays the line out within width. It is a pure function of the arguments
// and the meter's counters, which is the whole reason the layout is testable.
//
// A narrow window drops whole fields rather than slicing characters off the end.
// Cutting mid-number leaves "40.0M/12" or a dangling "et", which is worse than
// not showing the rate at all: a number you cannot trust is more misleading than
// a number that is absent.
func (m *Meter) render(width int, elapsed time.Duration) string {
	const gap = 2
	label := truncateLabel(m.label, meterMaxLabel)
	rate := ratePerSecond(m.done, elapsed)
	room := width - len([]rune(label)) - gap

	for _, right := range m.fields(rate) {
		rightWidth := len([]rune(right))
		if rightWidth > room {
			continue
		}
		// The widest set of fields that fits wins, and the bar takes whatever is
		// left: the numbers carry more than a longer bar would.
		if barWidth := room - rightWidth - gap; m.total > 0 && barWidth >= meterMinBar {
			filled := max(min(int(int64(barWidth)*m.done/m.total), barWidth), 0)
			bar := strings.Repeat(meterFilled, filled) + strings.Repeat(meterEmpty, barWidth-filled)
			return label + strings.Repeat(" ", gap) + bar + strings.Repeat(" ", gap) + right
		}
		return label + strings.Repeat(" ", gap) + right
	}

	// Not even the shortest field fits beside the label.
	return trimToWidth(label, width)
}

// fields returns the candidate right-hand sides, widest first, so that render can
// take the most informative one that fits.
//
// The order is a judgement about what a watcher needs: how far along, then how
// much of how much, then how fast, then how long is left. The estimate goes first
// because it is the one that can be recomputed by eye from the others.
func (m *Meter) fields(rate int64) []string {
	if m.total <= 0 {
		// No total, so no percentage that would not be invented, and no bar.
		return []string{
			fmt.Sprintf("%s  %s/s", HumanSize(m.done), HumanSize(rate)),
			HumanSize(m.done),
		}
	}

	pct := min(100*m.done/m.total, 100)
	head := fmt.Sprintf("%3d%%", pct)
	counts := fmt.Sprintf("%s/%s", HumanSize(m.done), HumanSize(m.total))
	speed := HumanSize(rate) + "/s"

	full := head + "  " + counts + "  " + speed
	out := make([]string, 0, 5)
	if eta, ok := etaOf(m.done, m.total, rate); ok {
		out = append(out, full+"  eta "+eta)
	}
	return append(out,
		full,
		head+"  "+counts,
		head+"  "+HumanSize(m.done),
		head,
	)
}

// truncateLabel shortens a name that would crowd out the numbers, keeping the
// end: the distinguishing part of "data/run3/2026/histograms.root" is the tail.
func truncateLabel(label string, maxLen int) string {
	r := []rune(label)
	if len(r) <= maxLen {
		return label
	}
	return "…" + string(r[len(r)-maxLen+1:])
}

// trimToWidth cuts a line that cannot fit. A bar that wraps turns one line into
// a screenful, which is much worse than one that is cut short.
func trimToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}

// ratePerSecond is the average rate so far. The average rather than a recent
// window: it is steadier to read, and for a transfer that is about to finish the
// average is the number that predicts the ETA correctly.
func ratePerSecond(done int64, elapsed time.Duration) int64 {
	if elapsed <= 0 || done <= 0 {
		return 0
	}
	return int64(float64(done) / elapsed.Seconds())
}

// etaOf estimates the time remaining, reporting false when there is nothing
// useful to say: no rate yet, or already finished.
func etaOf(done, total, rate int64) (string, bool) {
	if rate <= 0 || done >= total {
		return "", false
	}
	left := time.Duration(float64(total-done)/float64(rate)) * time.Second
	if left <= 0 {
		return "", false
	}
	return left.Round(time.Second).String(), true
}

// stderrFile is where the meter measures its width. It is stderr rather than
// stdout on purpose: a command such as "cat" writes the payload to stdout, which may be
// a pipe, and the bar has to lay itself out for the window a person is watching.
func stderrFile() *os.File { return os.Stderr }
