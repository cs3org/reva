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
	"github.com/cs3org/reva/pkg/mentix/util/registry"
)

var (
	registeredConnectors = registry.NewRegistry()
)

// Connector is the interface that all connectors must implement.
type Connector interface {
	// GetID returns the ID of the connector.
	GetID() string

	// Activate activates a connector.
	Activate(conf *config.Configuration, log *zerolog.Logger) error
	// RetrieveMeshData fetches new mesh data.
	RetrieveMeshData() (*meshdata.MeshData, error)

	// GetName returns the display name of the connector.
	GetName() string
}

// BaseConnector implements basic connector functionality common to all connectors.
type BaseConnector struct {
	id string

	conf *config.Configuration
	log  *zerolog.Logger
}

// GetID returns the ID of the connector.
func (connector *BaseConnector) GetID() string {
	return connector.id
}

// SetID sets the ID of the connector.
func (connector *BaseConnector) SetID(id string) {
	// The ID can only be set once
	if connector.id == "" {
		connector.id = id
	}
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
	entries, err := registeredConnectors.EntriesByID(conf.EnabledConnectors)
	if err != nil {
		return nil, err
	}

	connectors := make([]Connector, 0, len(entries))
	for _, entry := range entries {
		connectors = append(connectors, entry.(Connector))
	}

	// At least one connector must be configured
	if len(connectors) == 0 {
		return nil, fmt.Errorf("no connectors available")
	}

	return connectors, nil
}

func registerConnector(connector Connector) {
	registeredConnectors.Register(connector.GetID(), connector)
}
