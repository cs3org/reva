// Copyright 2018-2022 CERN
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

package antivirus

import (
	"io"
	"time"
)

// TODO: make configurable
var (
	_timeout      = 300 * time.Second
	_icapURL      = "icap://127.0.0.1:1344"
	_icapService  = "avscan"
	_clamavSocket = "/run/clamav/clamd.ctl" // "/tmp/clamd.socket"
)

// Scanner is an abstraction for the actual virus scan
type Scanner interface {
	Scan(file io.Reader) (ScanResult, error)
}

// ScanResult contains result about the scan
type ScanResult struct {
	Infected    bool
	Description string
}

// New returns an Antivirus
func New(typ string) (Scanner, error) {
	var (
		scanner Scanner
		err     error
	)

	switch typ {
	default:
		// TODO: error instead of fallback
		fallthrough
	case "clamav":
		scanner = NewClamAV(_clamavSocket)
	case "icap":
		scanner, err = NewICAP(_icapURL, _icapService, _timeout)
	}

	return scanner, err
}
