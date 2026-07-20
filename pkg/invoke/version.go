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

package invoke

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/cs3org/reva/v3/pkg/rversion"
)

// VersionInvocation is a built-in invocation every service instance exposes:
// the binary's build information and the process uptime, so a mixed fleet's
// version drift is visible with one fan-out.
const VersionInvocation = "version"

// processStart approximates the process start (package init runs at startup).
var processStart = time.Now()

func init() {
	registerDefault(defaultInvocation{
		spec: InvocationSpec{
			Name:        VersionInvocation,
			Description: "Return the binary's build information and the process uptime.",
			Kind:        KindReadonly,
		},
		fn: func(_ context.Context, _ instance, _ Args) (Result, error) {
			return versionResult(), nil
		},
	})
}

// versionResult reports the reva version (from the link-time metadata in
// pkg/rversion, populated by cmd/revad), falling back to the embedded build info
// for the commit/date when the binary was built without those ldflags.
func versionResult() Result {
	res := Result{
		"go":     runtime.Version(),
		"os":     runtime.GOOS + "/" + runtime.GOARCH,
		"pid":    os.Getpid(),
		"uptime": time.Since(processStart).Round(time.Second).String(),
	}
	// The reva release and build metadata, when the binary was stamped with them.
	if rversion.Version != "" {
		res["reva"] = rversion.Version
	}
	if rversion.GitCommit != "" {
		res["commit"] = shortCommit(rversion.GitCommit)
	}
	if rversion.BuildDate != "" {
		res["build_date"] = rversion.BuildDate
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return res
	}
	var revision, modified, vcstime string
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			revision = kv.Value
		case "vcs.modified":
			modified = kv.Value
		case "vcs.time":
			vcstime = kv.Value
		}
	}
	// Fall back to the vcs stamp only for what the ldflags did not provide.
	if _, ok := res["commit"]; !ok && revision != "" {
		commit := shortCommit(revision)
		if modified == "true" {
			commit += "-dirty"
		}
		res["commit"] = commit
	}
	if _, ok := res["build_date"]; !ok && vcstime != "" {
		res["build_date"] = vcstime
	}
	return res
}

// shortCommit trims a git revision to its short form.
func shortCommit(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
