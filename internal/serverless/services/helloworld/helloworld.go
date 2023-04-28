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

package helloworld

import (
	"fmt"
	"os"
	"time"

	"github.com/cs3org/reva/pkg/rserverless"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

type config struct {
	Outfile    string `mapstructure:"outfile"`
	DieTimeout int64  `mapstructure:"die_timeout"`
}

func (c *config) init() {
	if c.Outfile == "" {
		c.Outfile = "/tmp/revad-helloworld-hello"
	}
}

type svc struct {
	conf *config
	file *os.File
	log  *zerolog.Logger
}

func init() {
	rserverless.Register("helloworld", New)
}

// New returns a new helloworld service.
func New(m map[string]interface{}, log *zerolog.Logger) (rserverless.Service, error) {
	conf := &config{}
	conf.init()

	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(conf.Outfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Err(err)
		return nil, err
	}

	s := &svc{
		conf: conf,
		log:  log,
		file: file,
	}

	return s, nil
}

// Start starts the helloworld service.
func (s *svc) Start() {
	s.log.Debug().Msgf("helloworld server started with timeout %d, saying hello at %s", s.conf.DieTimeout, s.conf.Outfile)
	go s.sayHello(s.conf.Outfile)
}

func (s *svc) GracefulStop() error {
	s.log.Debug().Msgf("graceful stop requested, simulating delay of %d seconds", s.conf.DieTimeout)
	time.Sleep(time.Second * time.Duration(s.conf.DieTimeout))
	return s.file.Close()
}

// Stop stops the helloworld service.
func (s *svc) Stop() error {
	s.log.Debug().Msgf("hard stop requested")
	return s.file.Close()
}

func (s *svc) sayHello(filename string) {
	for {
		s.log.Info().Msg("saying hello")
		h := fmt.Sprintf("%s - hello world!\n", time.Now().String())

		_, err := s.file.Write([]byte(h))
		if err != nil {
			s.log.Err(err)
		}
		time.Sleep(5 * time.Second)
	}
}
