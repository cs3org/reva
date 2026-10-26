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

package cli

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/cli/data"
)

// TransferSettings are the defaults of the transfers: [transfer] of the
// user config, else of the brand.
type TransferSettings struct {
	ChunkSize int64
	// Archive lets a recursive download use the archiver.
	Archive bool
	// Jobs is the number of parallel transfers. Zero is the default.
	Jobs int
}

// Transfer returns the transfer settings.
func (r *Runtime) Transfer() TransferSettings {
	if !r.resolved {
		r.resolve()
	}
	b, c := r.Brand().Transfer, r.config.Transfer
	t := TransferSettings{ChunkSize: r.chunkSize(), Archive: true, Jobs: c.Jobs}
	if b.Archive != nil {
		t.Archive = *b.Archive
	}
	if c.Archive != nil {
		t.Archive = *c.Archive
	}
	return t
}

func (r *Runtime) chunkSize() int64 {
	if n, err := ParseSize(cmp.Or(r.config.Transfer.ChunkSize, r.Brand().Transfer.ChunkSize)); err == nil && n > 0 {
		return n
	}
	return data.DefaultChunkSize
}

// ParseSize reads a size with an optional K, M or G suffix (powers of
// 1024), for example 8M.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	if s != "" {
		switch strings.ToUpper(s[len(s)-1:]) {
		case "K":
			mult = 1 << 10
		case "M":
			mult = 1 << 20
		case "G":
			mult = 1 << 30
		}
		if mult > 1 {
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, Usagef("size %q: use a number with an optional K, M or G", s)
	}
	return n * mult, nil
}
