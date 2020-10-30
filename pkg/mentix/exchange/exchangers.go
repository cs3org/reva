// Copyright 2018-2020 CERN
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

package exchange

import (
	"fmt"

	"github.com/cs3org/reva/pkg/mentix/config"

	"github.com/rs/zerolog"
)

// ActivateExchangers activates the given exchangers.
func ActivateExchangers(exchangers []Exchanger, conf *config.Configuration, log *zerolog.Logger) error {
	for _, exchanger := range exchangers {
		if err := exchanger.Activate(conf, log); err != nil {
			return fmt.Errorf("unable to activate exchanger '%v': %v", exchanger.GetName(), err)
		}
	}

	return nil
}

// StartExchangers starts the given exchangers.
func StartExchangers(exchangers []Exchanger) error {
	for _, exchanger := range exchangers {
		if err := exchanger.Start(); err != nil {
			return fmt.Errorf("unable to start exchanger '%v': %v", exchanger.GetName(), err)
		}
	}

	return nil
}

// StopExchangers stops the given exchangers.
func StopExchangers(exchangers []Exchanger) {
	for _, exchanger := range exchangers {
		exchanger.Stop()
	}
}

// GetRequestExchangers gets all exchangers from a vector that implement the RequestExchanger interface.
func GetRequestExchangers(exchangers []Exchanger) []RequestExchanger {
	var reqExchangers []RequestExchanger
	for _, exporter := range exchangers {
		if reqExchanger, ok := exporter.(RequestExchanger); ok {
			reqExchangers = append(reqExchangers, reqExchanger)
		}
	}
	return reqExchangers
}
