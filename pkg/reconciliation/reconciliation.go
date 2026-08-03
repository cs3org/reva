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

package reconciliation

import (
	"os"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Config configures the reconciliation jobs. It is decoded from the job's own
// configuration section and its values are set on the job at wiring time.
type Config struct {
	// DryRun, when set, makes the job log and report what it would do without
	// touching the share database.
	DryRun bool `mapstructure:"dry_run"`
	// LogFile is the path the job writes its own log to. It takes "stdout" or
	// "stderr" to write to the standard streams instead.
	LogFile string `mapstructure:"log_file"`
}

// ApplyDefaults fills in the unset fields. It is not called automatically when
// the struct is embedded, so an embedding config has to call it.
func (c *Config) ApplyDefaults() {
	if c.LogFile == "" {
		c.LogFile = "/var/log/revad/reconciliation.log"
	}
}

// OpenLog opens the job's own log. The job keeps its own log rather than
// writing to revad's because the log is the record of what a run changed: it is
// always JSON, whatever [log] mode the rest of the process runs in, and it is
// not interleaved with unrelated output.
//
// The returned file is nil when the log goes to a standard stream, and is the
// caller's to close otherwise.
func OpenLog(path string) (*zerolog.Logger, *os.File, error) {
	switch path {
	case "stdout":
		log := zerolog.New(os.Stdout).With().Timestamp().Logger()
		return &log, nil, nil
	case "stderr":
		log := zerolog.New(os.Stderr).With().Timestamp().Logger()
		return &log, nil, nil
	}

	// the log is opened at startup so that a bad path is a startup error
	// rather than a run that silently keeps no record.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "reconciliation: opening the log at %s", path)
	}
	log := zerolog.New(f).With().Timestamp().Logger()
	return &log, f, nil
}
