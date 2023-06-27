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
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Service represents a serverless service.
type Service interface {
	Start()
	Close(ctx context.Context) error
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
	log      *zerolog.Logger
	Services map[string]Service
}

// New returns a new serverless collection of services.
func New(opt ...Option) (*Serverless, error) {
	l := zerolog.Nop()
	n := &Serverless{
		Services: map[string]Service{},
		log:      &l,
	}
	for _, o := range opt {
		o(n)
	}
	return n, nil
}

// Start starts the serverless service collection.
func (s *Serverless) Start() error {
	for name, svc := range s.Services {
		go svc.Start()
		s.log.Info().Msgf("serverless service enabled: %s", name)
	}
	return nil
}

// GracefulStop gracefully stops the serverless services.
func (s *Serverless) GracefulStop() error {
	var wg sync.WaitGroup

	for svcName, svc := range s.Services {
		wg.Add(1)

		go func(svcName string, svc Service) {
			defer wg.Done()

			s.log.Info().Msgf("Sending stop request to service %s", svcName)
			ctx := context.Background()

			err := svc.Close(ctx)
			if err != nil {
				s.log.Error().Err(err).Msgf("error stopping service %s", svcName)
			} else {
				s.log.Info().Msgf("service %s stopped", svcName)
			}
		}(svcName, svc)
	}

	wg.Wait()

	return nil
}

// Stop stops the serverless services with a one second deadline.
func (s *Serverless) Stop() error {
	var wg sync.WaitGroup

	for svcName, svc := range s.Services {
		wg.Add(1)

		go func(svcName string, svc Service) {
			defer wg.Done()

			s.log.Info().Msgf("Sending stop request to service %s", svcName)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			err := svc.Close(ctx)
			if err != nil {
				s.log.Error().Err(err).Msgf("error stopping service %s", svcName)
			} else {
				s.log.Info().Msgf("service %s stopped", svcName)
			}
		}(svcName, svc)
	}

	wg.Wait()

	return nil
}

