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

package connectors

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

var (
	registeredConnectors = map[string]Connector{}
)

// Connector is the interface that all connectors must implement.
type Connector interface {
	// Activate activates a connector.
	Activate(conf *config.Configuration, log *zerolog.Logger) error
	// RetrieveMeshData fetches new mesh data.
	RetrieveMeshData() (*meshdata.MeshData, error)

	// GetName returns the display name of the connector.
	GetName() string
}

// BaseConnector implements basic connector functionality common to all connectors.
type BaseConnector struct {
	conf *config.Configuration
	log  *zerolog.Logger
}

// Activate activates the connector.
func (connector *BaseConnector) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	connector.conf = conf

	if log == nil {
		return fmt.Errorf("no logger provided")
	}
	connector.log = log

	return nil
}

// AvailableConnectors returns a list of all connectors that are enabled in the configuration.
func AvailableConnectors(conf *config.Configuration) ([]Connector, error) {
	// Try to add all connectors configured in the environment
	var connectors []Connector
	for _, connectorID := range conf.EnabledConnectors {
		if connector, ok := registeredConnectors[connectorID]; ok {
			connectors = append(connectors, connector)
		} else {
			return nil, fmt.Errorf("no connector with ID '%v' registered", connectorID)
		}
	}

	// At least one connector must be configured
	if len(connectors) == 0 {
		return nil, fmt.Errorf("no connectors available")
	}

	return connectors, nil
}

func registerConnector(id string, connector Connector) {
	registeredConnectors[id] = connector
}
