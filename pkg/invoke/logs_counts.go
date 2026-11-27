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
	"flag"
	"os"

	"github.com/cs3org/reva/v3/pkg/logger"
)

// LogCountsInvocation is a built-in invocation every service instance
// exposes: the process's counts of log messages at warn level or above, so a
// collector can read them across the fleet without scraping every process.
// The counts are process-wide; pid and start tell instances of one process
// apart from a restarted one.
const LogCountsInvocation = "logcounts"

func init() {
	registerDefault(
		InvocationSpec{
			Name:        LogCountsInvocation,
			Description: "Return the process's counts of log messages at warn level or above.",
			Kind:        KindReadonly,
		},
		func(_ context.Context, _ instance, _ struct{}) (Result, error) {
			return Result{
				"counts": logger.MessageCounts(),
				"pid":    os.Getpid(),
				"start":  processStart.Unix(),
				"config": configFile(),
			}, nil
		},
		nil,
	)
}

// configFile is revad's -c flag: the process's configuration file names it
// the way its log file does.
func configFile() string {
	if f := flag.Lookup("c"); f != nil {
		return f.Value.String()
	}
	return ""
}
