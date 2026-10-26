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
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// resizePollInterval is how often the console size is checked. Short enough
// that a redraw follows a drag of the window edge without visible lag, long
// enough to cost nothing.
const resizePollInterval = 200 * time.Millisecond

// watchResize calls fire whenever the console changes size, until stop is
// called.
//
// Windows has no SIGWINCH. The console reports a resize as an input record, but
// the screen reads keys with ReadFile, which only ever returns the characters
// and drops every other record: so the size is polled instead.
func watchResize(out *os.File, fire func()) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(resizePollInterval)
		defer ticker.Stop()
		w, h, _ := term.GetSize(int(out.Fd()))
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				nw, nh, err := term.GetSize(int(out.Fd()))
				if err != nil || (nw == w && nh == h) {
					continue
				}
				w, h = nw, nh
				fire()
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
