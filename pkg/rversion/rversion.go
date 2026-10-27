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

// Package rversion holds the version of the build, for revad and for the CLI.
// The build sets it with -X. The fields are empty without those flags.
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
