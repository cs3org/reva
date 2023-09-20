// Copyright 2018-2021 CERN
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

package main

import (
	"os"

	"github.com/cs3org/reva/v2/pkg/micro/ocdav"
	"github.com/rs/zerolog"
)

// main starts a go micro service that uses the ocdev handler
// It is an example how to use pkg/micro/ocdav Service and leaves out any flag parsing.
func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	s, err := ocdav.Service(
		ocdav.Logger(logger),
		ocdav.GatewaySvc("127.0.0.1:9142"),
		ocdav.FilesNamespace("/users/{{.Id.OpaqueId}}"),
		ocdav.WebdavNamespace("/users/{{.Id.OpaqueId}}"),
		ocdav.OCMNamespace("/public"),
		ocdav.SharesNamespace("/Shares"),
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed starting ocdav service")
		return
	}
	if err := s.Run(); err != nil {
		logger.Fatal().Err(err).Msg("ocdav service exited with error")
	}
}
