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
	"strings"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

var (
	registeredConnectors = map[string]Connector{}
)

type Connector interface {
	Activate(conf *config.Configuration, log *zerolog.Logger) error
	RetrieveMeshData() (*meshdata.MeshData, error)

	GetName() string
}

type BaseConnector struct {
	conf *config.Configuration
	log  *zerolog.Logger
}

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

func FindConnector(connectorID string) (Connector, error) {
	for id, connector := range registeredConnectors {
		if strings.EqualFold(id, connectorID) {
			return connector, nil
		}
	}

	return nil, fmt.Errorf("no connector with ID '%v' registered", connectorID)
}

func registerConnector(id string, connector Connector) {
	registeredConnectors[id] = connector
}
