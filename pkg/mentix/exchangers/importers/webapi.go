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

package importers

import (
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exchangers/importers/webapi"
)

// WebAPIImporter implements the generic Web API importer.
type WebAPIImporter struct {
	BaseRequestImporter
}

// Activate activates the importer.
func (exporter *WebAPIImporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := exporter.BaseRequestImporter.Activate(conf, log); err != nil {
		return err
	}

	// Store WebAPI specifics
	exporter.SetEndpoint(conf.Importers.WebAPI.Endpoint)
	exporter.SetEnabledConnectors(conf.Importers.WebAPI.EnabledConnectors)

	exporter.registerSiteActionHandler = webapi.HandleRegisterSiteQuery
	exporter.unregisterSiteActionHandler = webapi.HandleUnregisterSiteQuery

	return nil
}

// GetID returns the ID of the importer.
func (exporter *WebAPIImporter) GetID() string {
	return config.ImporterIDWebAPI
}

// GetName returns the display name of the importer.
func (exporter *WebAPIImporter) GetName() string {
	return "WebAPI"
}

func init() {
	registerImporter(&WebAPIImporter{})
}
