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

package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// uploadState is what makes a resumable upload survive the CLI exiting. Without
// it, an upload interrupted at 48 GB of 50 would start again from zero on the
// next invocation, which is the difference between resumable uploads being a
// feature and being a detail of one process's lifetime.
type uploadState struct {
	Upload     *backend.Upload `json:"upload"`
	LocalSize  int64           `json:"local_size"`
	LocalMtime time.Time       `json:"local_mtime"`
	Offset     int64           `json:"offset"`
	SavedAt    time.Time       `json:"saved_at"`
}

// stateTTL bounds how long a partial upload is worth resuming. Servers expire
// abandoned uploads, so a very old record is more likely to waste a round trip
// than to save one.
const stateTTL = 7 * 24 * time.Hour

func (e *Engine) statePath(localPath, remotePath string) string {
	sum := sha256.Sum256([]byte(localPath + "\x00" + remotePath))
	return filepath.Join(e.opts.StateDir, hex.EncodeToString(sum[:])+".json")
}

// loadState returns a previously recorded upload, if it is still applicable to
// this local file.
func (e *Engine) loadState(localPath, remotePath string, info os.FileInfo) (*backend.Upload, bool) {
	data, err := os.ReadFile(e.statePath(localPath, remotePath))
	if err != nil {
		return nil, false
	}
	var st uploadState
	if err := json.Unmarshal(data, &st); err != nil || st.Upload == nil {
		return nil, false
	}

	// The local file must be the same one the upload was started for.
	// Resuming after an edit would splice two different files together, and
	// the result would pass every length check while being silently wrong.
	if st.LocalSize != info.Size() || !st.LocalMtime.Equal(info.ModTime()) {
		return nil, false
	}
	if time.Since(st.SavedAt) > stateTTL {
		return nil, false
	}

	st.Upload.Offset = st.Offset
	return st.Upload, true
}

func (e *Engine) saveState(localPath, remotePath string, up *backend.Upload, offset int64) {
	info, err := os.Stat(localPath)
	if err != nil {
		return
	}
	st := uploadState{
		Upload:     up,
		LocalSize:  info.Size(),
		LocalMtime: info.ModTime(),
		Offset:     offset,
		SavedAt:    time.Now(),
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(e.opts.StateDir, 0o700); err != nil {
		return
	}
	// The state file records an upload URL, which is a capability: anyone
	// holding it can append to the upload. 0600 keeps it to the owner.
	// A failure here is not worth failing the transfer over: the upload
	// simply will not be resumable.
	_ = os.WriteFile(e.statePath(localPath, remotePath), data, 0o600)
}

func (e *Engine) clearState(localPath, remotePath string) {
	_ = os.Remove(e.statePath(localPath, remotePath))
}
