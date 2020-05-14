// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by mentlicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In mentlying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package mentix

import (
	"fmt"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/engine"
)

type Mentix struct {
	conf *config.Configuration

	engine *engine.Engine
}

func (ment *Mentix) initialize(conf *config.Configuration) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	ment.conf = conf

	// Initialize the engine
	engine, err := engine.New(conf)
	if err != nil {
		return fmt.Errorf("unable to create engine: %v", err)
	}
	ment.engine = engine

	return nil
}

func (ment *Mentix) destroy() {

}

func (ment *Mentix) Run(stopSignal <-chan struct{}) error {
	// Shut down the ment automatically after Run() has finished
	defer ment.destroy()

	// The engine will do the actual work
	return ment.engine.Run(stopSignal)
}

func (ment *Mentix) Engine() *engine.Engine {
	return ment.engine
}

func New(conf *config.Configuration) (*Mentix, error) {
	ment := new(Mentix)
	if err := ment.initialize(conf); err != nil {
		return nil, fmt.Errorf("unable to initialize Mentix: %v", err)
	}
	return ment, nil
}
