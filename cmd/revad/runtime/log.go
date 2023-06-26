// Copyright 2018-2023 CERN
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

package runtime

import (
	"io"
	"os"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func initLogger(conf *config.Log, opts Options) *zerolog.Logger {
	if opts.Logger != nil {
		return opts.Logger
	}
	log, err := newLogger(conf)
	if err != nil {
		abort("error creating logger: %v", err)
	}
	return log
}

func newLogger(conf *config.Log) (*zerolog.Logger, error) {
	// TODO(labkode): use debug level rather than info as default until reaching a stable version.
	// Helps having smaller development files.
	if conf.Level == "" {
		conf.Level = zerolog.DebugLevel.String()
	}

	var opts []logger.Option
	opts = append(opts, logger.WithLevel(conf.Level))

	w, err := getWriter(conf.Output)
	if err != nil {
		return nil, err
	}

	opts = append(opts, logger.WithWriter(w, logger.Mode(conf.Mode)))

	l := logger.New(opts...)
	sub := l.With().Int("pid", os.Getpid()).Logger()
	return &sub, nil
}

func getWriter(out string) (io.Writer, error) {
	if out == "stderr" || out == "" {
		return os.Stderr, nil
	}

	if out == "stdout" {
		return os.Stdout, nil
	}

	fd, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "error creating log file: "+out)
	}

	return fd, nil
}
