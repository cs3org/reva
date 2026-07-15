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

// versionResult reads the embedded build info: the module version, the vcs
// revision (with a -dirty suffix for a modified tree) and its commit time.
// Binaries built without vcs stamping (-buildvcs=false) report what they have.
func versionResult() Result {
	res := Result{
		"go":     runtime.Version(),
		"os":     runtime.GOOS + "/" + runtime.GOARCH,
		"pid":    os.Getpid(),
		"uptime": time.Since(processStart).Round(time.Second).String(),
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return res
	}
	if bi.Main.Version != "" {
		res["version"] = bi.Main.Version
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
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if modified == "true" {
			revision += "-dirty"
		}
		res["commit"] = revision
	}
	if vcstime != "" {
		res["commit_time"] = vcstime
	}
	return res
}
