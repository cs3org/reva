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

package rserverless

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Service represents a serverless service.
type Service interface {
	Start()
}

// Services is a map of service name and its new function.
var Services = map[string]NewService{}

// Register registers a new serverless service with name and new function.
func Register(name string, newFunc NewService) {
	Services[name] = newFunc
}

// NewService is the function that serverless services need to register at init time.
type NewService func(conf map[string]interface{}, log *zerolog.Logger) (Service, error)

// Serverless contains the serveless collection of services.
type Serverless struct {
	conf     *config
	log      zerolog.Logger
	services map[string]Service
}

type config struct {
	Services map[string]map[string]interface{} `mapstructure:"services"`
}

// New returns a new serverless collection of services.
func New(m interface{}, l zerolog.Logger) (*Serverless, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	n := &Serverless{
		conf:     conf,
		log:      l,
		services: map[string]Service{},
	}
	return n, nil
}

func (s *Serverless) isServiceEnabled(svcName string) bool {
	_, ok := Services[svcName]
	return ok
}

// Start starts the serverless service collection.
func (s *Serverless) Start() error {
	return s.registerServices()
}

func (s *Serverless) registerServices() error {
	for svcName := range s.conf.Services {
		if s.isServiceEnabled(svcName) {
			newFunc := Services[svcName]
			svcLogger := s.log.With().Str("service", svcName).Logger()
			svc, err := newFunc(s.conf.Services[svcName], &svcLogger)
			if err != nil {
				return errors.Wrapf(err, "serverless service %s could not be initialized", svcName)
			}

			go svc.Start()

			s.services[svcName] = svc

			s.log.Info().Msgf("serverless service enabled: %s", svcName)
		} else {
			message := fmt.Sprintf("serverless service %s does not exist", svcName)
			return errors.New(message)
		}
	}

	return nil
}
