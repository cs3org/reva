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

// Package output has the progress meter and the size format of the CLI.
package output

import "fmt"

const (
	_ = 1 << (10 * iota)
	kib
	mib
	gib
	tib
	pib
)

// HumanSize renders a byte count the way ls -h does.
func HumanSize(n int64) string {
	switch {
	case n < kib:
		return fmt.Sprintf("%d", n)
	case n < mib:
		return fmt.Sprintf("%.1fK", float64(n)/kib)
	case n < gib:
		return fmt.Sprintf("%.1fM", float64(n)/mib)
	case n < tib:
		return fmt.Sprintf("%.1fG", float64(n)/gib)
	case n < pib:
		return fmt.Sprintf("%.1fT", float64(n)/tib)
	default:
		return fmt.Sprintf("%.1fP", float64(n)/pib)
	}
}
