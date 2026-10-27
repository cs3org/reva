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

type uploadState struct {
	Upload     *backend.Upload `json:"upload"`
	LocalSize  int64           `json:"local_size"`
	LocalMtime time.Time       `json:"local_mtime"`
	SavedAt    time.Time       `json:"saved_at"`
}

// stateTTL is how long a partial upload is worth resuming. Servers expire
// abandoned uploads.
const stateTTL = 7 * 24 * time.Hour

func (e *Engine) statePath(localPath, remotePath string) string {
	sum := sha256.Sum256([]byte(localPath + "\x00" + remotePath))
	return filepath.Join(e.opts.StateDir, hex.EncodeToString(sum[:])+".json")
}

func (e *Engine) loadState(localPath, remotePath string, info os.FileInfo) (*backend.Upload, bool) {
	data, err := os.ReadFile(e.statePath(localPath, remotePath))
	if err != nil {
		return nil, false
	}
	var st uploadState
	if err := json.Unmarshal(data, &st); err != nil || st.Upload == nil {
		return nil, false
	}

	// After an edit, resuming would splice two files and pass every length check.
	if st.LocalSize != info.Size() || !st.LocalMtime.Equal(info.ModTime()) {
		return nil, false
	}
	if time.Since(st.SavedAt) > stateTTL {
		return nil, false
	}
	return st.Upload, true
}

func (e *Engine) saveState(localPath, remotePath string, up *backend.Upload) {
	info, err := os.Stat(localPath)
	if err != nil {
		return
	}
	st := uploadState{
		Upload:     up,
		LocalSize:  info.Size(),
		LocalMtime: info.ModTime(),
		SavedAt:    time.Now(),
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(e.opts.StateDir, 0o700); err != nil {
		return
	}
	// The upload URL lets anyone append to the upload: keep the file private.
	// A failure only makes the upload not resumable.
	_ = os.WriteFile(e.statePath(localPath, remotePath), data, 0o600)
}

func (e *Engine) clearState(localPath, remotePath string) {
	_ = os.Remove(e.statePath(localPath, remotePath))
}
