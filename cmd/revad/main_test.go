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

package revadcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// What logrotate does without copytruncate: rename the log, then SIGUSR2.
func TestLogReopenedOnSIGUSR2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "revad.log")
	w, err := getWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(w, "before")
	if err := os.Rename(path, path+"-1"); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}

	// The reopen is asynchronous: wait until the descriptor is the new file.
	for i := 0; ; i++ {
		cur, err1 := w.(*os.File).Stat()
		want, err2 := os.Stat(path)
		if err1 == nil && err2 == nil && os.SameFile(cur, want) {
			break
		}
		if i == 100 {
			t.Fatal("log file not reopened")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Fprintln(w, "after")

	for file, want := range map[string]string{path + "-1": "before\n", path: "after\n"} {
		if got, err := os.ReadFile(file); err != nil || string(got) != want {
			t.Errorf("%s = %q (%v), want %q", file, got, err, want)
		}
	}
}
