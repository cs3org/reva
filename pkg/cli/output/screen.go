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
	"unicode/utf8"

	"golang.org/x/term"
)

// Line is one row of a full-screen frame.
//
// Highlight is a whole-line attribute rather than styled text inside Text
// because the screen has to know how wide a line is in order to trim it, and it
// cannot know that for a string carrying escape sequences. Restricting styling
// to entire lines keeps the widths honest.
type Line struct {
	Text      string
	Highlight bool
}

// Screen is a full-screen terminal that a browser draws frames into.
//
// It is an interface so that the thing driving it can be tested without a
// terminal: a fake supplies a size and a scripted sequence of events, and
// records the frames it is asked to draw.
type Screen interface {
	// Size is the current width and height in cells.
	Size() (width, height int)
	// NextEvent blocks until the user presses a key or the terminal is resized.
	NextEvent() (Event, error)
	// Draw replaces the visible frame.
	Draw(lines []Line) error
	// Close restores the terminal to the state it was found in.
	Close() error
}

// Event is something the user did.
type Event struct {
	Key Key
	// Resize reports that the terminal changed size; Key is then meaningless.
	Resize bool
}

// KeyName names a key that is not a printable character.
type KeyName int

// The named keys. KeyRune means the press is in Key.Rune instead.
const (
	KeyRune KeyName = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInterrupt
)

// Key is one keypress.
type Key struct {
	Name KeyName
	Rune rune
}

// Is reports whether the key is the given character.
func (k Key) Is(r rune) bool { return k.Name == KeyRune && k.Rune == r }

// Terminal control sequences. The alternate screen buffer is what lets the
// browser take the whole terminal and give back exactly what was there before.
const (
	altScreenOn  = "\033[?1049h"
	altScreenOff = "\033[?1049l"
	cursorHide   = "\033[?25l"
	cursorShow   = "\033[?25h"
	reverseOn    = "\033[7m"
	attrOff      = "\033[0m"
)

type termScreen struct {
	in    *os.File
	out   *os.File
	state *term.State

	events     chan Event
	stopResize func()

	closeOnce sync.Once
}

// OpenScreen takes over the terminal.
//
// It fails rather than half-works when either end is not a terminal: a frame
// written to a pipe is gibberish, and raw mode on a file that is not a terminal
// leaves nothing to read keys from.
func OpenScreen(in, out *os.File) (Screen, error) {
	if in == nil || out == nil || !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		return nil, fmt.Errorf("not a terminal")
	}

	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}

	s := &termScreen{
		in:     in,
		out:    out,
		state:  state,
		events: make(chan Event, 64),
	}

	fmt.Fprint(out, altScreenOn+cursorHide)

	// A terminal left in raw mode with no cursor is unusable, and the shell
	// that gets it back has no way to know it needs fixing. Every path out ,
	// a normal quit, a signal, a panic in the caller: has to go through Close,
	// so the signals that would otherwise kill the process silently are caught
	// and turned into an ordinary interrupt event.
	s.stopResize = watchResize(out, s.resized)
	go s.readKeys()

	return s, nil
}

// resized queues a resize event, dropping it when one is already waiting: a
// redraw reads the size afresh, so two in a row draw the same frame twice.
func (s *termScreen) resized() {
	select {
	case s.events <- Event{Resize: true}:
	default:
	}
}

// readKeys parses the byte stream into events.
//
// One Read can return a whole escape sequence, several keypresses, or part of
// one: terminals write a sequence in a single go, so a chunk is parsed as a
// whole rather than byte by byte with a timer to tell ESC from ESC[A. The
// goroutine is left blocked in Read when the screen closes, which is fine
// because the browser is the last thing the process does.
func (s *termScreen) readKeys() {
	buf := make([]byte, 128)
	for {
		n, err := s.in.Read(buf)
		if n > 0 {
			for _, k := range ParseKeys(buf[:n]) {
				s.events <- Event{Key: k}
			}
		}
		if err != nil {
			close(s.events)
			return
		}
	}
}

func (s *termScreen) NextEvent() (Event, error) {
	ev, ok := <-s.events
	if !ok {
		return Event{}, fmt.Errorf("input closed")
	}
	return ev, nil
}

func (s *termScreen) Size() (int, int) {
	w, h, err := term.GetSize(int(s.out.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func (s *termScreen) Draw(lines []Line) error {
	w, h := s.Size()

	var sb strings.Builder
	sb.WriteString("\033[H")
	for row := range h {
		fmt.Fprintf(&sb, "\033[%d;1H\033[2K", row+1)
		if row >= len(lines) {
			continue
		}
		text := Ellipsize(lines[row].Text, w)
		if lines[row].Highlight {
			// Padded to the full width first, so the highlight covers the row
			// rather than stopping at the last character.
			text += strings.Repeat(" ", max(0, w-utf8.RuneCountInString(text)))
			sb.WriteString(reverseOn + text + attrOff)
			continue
		}
		sb.WriteString(text)
	}
	_, err := io.WriteString(s.out, sb.String())
	return err
}

func (s *termScreen) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.stopResize()
		fmt.Fprint(s.out, cursorShow+altScreenOff)
		err = term.Restore(int(s.in.Fd()), s.state)
	})
	return err
}

// ParseKeys decodes a chunk of terminal input.
func ParseKeys(buf []byte) []Key {
	var keys []Key
	for i := 0; i < len(buf); {
		b := buf[i]
		switch {
		case b == 0x1b:
			k, used := parseEscape(buf[i:])
			keys = append(keys, k)
			i += used
		case b == '\r' || b == '\n':
			keys = append(keys, Key{Name: KeyEnter})
			i++
		case b == 0x7f || b == 0x08:
			keys = append(keys, Key{Name: KeyBackspace})
			i++
		case b == '\t':
			keys = append(keys, Key{Name: KeyTab})
			i++
		case b == 0x03:
			keys = append(keys, Key{Name: KeyInterrupt})
			i++
		case b < 0x20:
			// Any other control character is ignored rather than delivered as a
			// stray rune that would land in a search box.
			i++
		default:
			r, size := utf8.DecodeRune(buf[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			keys = append(keys, Key{Name: KeyRune, Rune: r})
			i += size
		}
	}
	return keys
}

// parseEscape reads one escape sequence, returning the key and how many bytes
// it consumed. A lone ESC is the Escape key.
func parseEscape(buf []byte) (Key, int) {
	if len(buf) < 3 || (buf[1] != '[' && buf[1] != 'O') {
		return Key{Name: KeyEscape}, 1
	}
	switch buf[2] {
	case 'A':
		return Key{Name: KeyUp}, 3
	case 'B':
		return Key{Name: KeyDown}, 3
	case 'C':
		return Key{Name: KeyRight}, 3
	case 'D':
		return Key{Name: KeyLeft}, 3
	case 'H':
		return Key{Name: KeyHome}, 3
	case 'F':
		return Key{Name: KeyEnd}, 3
	}
	// The numeric forms: ESC [ <n> ~
	if len(buf) >= 4 && buf[3] == '~' {
		switch buf[2] {
		case '1', '7':
			return Key{Name: KeyHome}, 4
		case '4', '8':
			return Key{Name: KeyEnd}, 4
		case '5':
			return Key{Name: KeyPageUp}, 4
		case '6':
			return Key{Name: KeyPageDown}, 4
		}
		return Key{Name: KeyEscape}, 4
	}
	return Key{Name: KeyEscape}, 1
}

// Ellipsize shortens s to at most width cells, marking the cut with an ellipsis,
// and counts runes rather than bytes so that a non-ASCII name does not push a
// frame out of shape.
//
// Distinct from trimToWidth, which cuts without a mark: a progress bar reads
// better cut cleanly, while a truncated file name has to say it was truncated or
// it reads as a different file.
func Ellipsize(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	out := make([]rune, 0, width)
	for _, r := range s {
		if len(out) == width-1 {
			break
		}
		out = append(out, r)
	}
	return string(out) + "…"
}
