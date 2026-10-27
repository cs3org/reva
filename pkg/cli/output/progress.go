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

	"golang.org/x/term"
)

// Meter draws the progress of a transfer on one line, rewriting it in place.
// Add and Set take events of the transfer engine. Writer wraps a plain
// io.Copy.
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
	meterFilled = "█"
	meterEmpty  = "░"

	meterRedraw = 100 * time.Millisecond

	// Below meterMinBar the bar is dropped and the numbers keep the line.
	meterMinBar = 8

	// meterMaxLabel keeps a deep path from crowding out the numbers.
	meterMaxLabel = 28
)

type MeterOption func(*Meter)

// MeterWidth fixes the width of the bar. Without it, the meter asks the
// terminal on every draw, so that it follows a resize.
func MeterWidth(n int) MeterOption { return func(m *Meter) { m.width = n } }

// MeterClock replaces the clock, for tests.
func MeterClock(f func() time.Time) MeterOption { return func(m *Meter) { m.now = f } }

// NewMeter returns a Meter drawing to out. A total of -1 means the size is
// not known: the meter shows a byte count and a rate, without a bar.
func NewMeter(out io.Writer, label string, total int64, opts ...MeterOption) *Meter {
	m := &Meter{out: out, label: label, total: total, now: time.Now}
	for _, o := range opts {
		o(m)
	}
	m.start = m.now()
	// Backdated, so that the first update draws at once.
	m.lastDraw = m.start.Add(-meterRedraw)
	return m
}

// SetLabel names the item that is being transferred now.
func (m *Meter) SetLabel(label string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.label = label
}

// Add records transferred bytes.
func (m *Meter) Add(n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done += n
	m.draw(false)
}

// Set records an absolute position. A total of zero keeps the known total.
func (m *Meter) Set(done, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done = done
	if total > 0 {
		m.total = total
	}
	m.draw(false)
}

// Stop erases the line.
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

// Writer returns dst wrapped so that what is written through it counts
// towards the meter.
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

// draw runs with the lock held.
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

	// Pad over the previous line instead of using an escape sequence, so that
	// tests can compare the output.
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
	// The width of stderr, not of stdout: stdout can be a pipe with the payload.
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

// render lays the line out within width. A narrow window drops whole fields
// instead of cutting a number in the middle.
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
		// The widest set of fields that fits wins. The bar takes the rest.
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

// fields returns the candidate right-hand sides, widest first.
func (m *Meter) fields(rate int64) []string {
	if m.total <= 0 {
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

// truncateLabel keeps the end of a name, which tells paths apart.
func truncateLabel(label string, maxLen int) string {
	r := []rune(label)
	if len(r) <= maxLen {
		return label
	}
	return "…" + string(r[len(r)-maxLen+1:])
}

// trimToWidth cuts a line that cannot fit, because a line that wraps breaks
// the in-place redraw.
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

// ratePerSecond is the average rate so far, which is steadier to read than a
// recent window.
func ratePerSecond(done int64, elapsed time.Duration) int64 {
	if elapsed <= 0 || done <= 0 {
		return 0
	}
	return int64(float64(done) / elapsed.Seconds())
}

// etaOf estimates the time left. It reports false with no rate or when done.
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
