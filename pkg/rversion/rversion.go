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

// Package rversion holds reva's build/version metadata. The values are set at
// link time on cmd/revad (see the Makefile ldflags) and mirrored here at startup
// so any package — e.g. the `version` invocation in pkg/invoke — can report the
// reva version without importing cmd/revad (which would be an import cycle).
// Fields are empty in binaries built without those ldflags.
package rversion

var (
	// Version is the reva release, from the repo VERSION file (e.g. "3.10.4").
	Version string
	// GitCommit is the commit the binary was built from.
	GitCommit string
	// BuildDate is when the binary was built.
	BuildDate string
	// GoVersion is the Go toolchain version used to build it.
	GoVersion string
)
