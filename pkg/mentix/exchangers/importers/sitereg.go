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

package importers

import (
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exchangers/importers/sitereg"
)

// SiteRegistrationImporter implements the external site registration importer.
type SiteRegistrationImporter struct {
	BaseRequestImporter
}

// Activate activates the importer.
func (importer *SiteRegistrationImporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := importer.BaseRequestImporter.Activate(conf, log); err != nil {
		return err
	}

	// Store SiteRegistration specifics
	importer.SetEndpoint(conf.Importers.SiteRegistration.Endpoint, conf.Importers.SiteRegistration.IsProtected)
	importer.SetEnabledConnectors(conf.Importers.SiteRegistration.EnabledConnectors)
	importer.SetAllowUnauthorizedSites(true)

	importer.RegisterExtendedActionHandler("register", sitereg.HandleRegisterSiteQuery)
	importer.RegisterExtendedActionHandler("unregister", sitereg.HandleUnregisterSiteQuery)

	return nil
}

// GetID returns the ID of the importer.
func (importer *SiteRegistrationImporter) GetID() string {
	return config.ImporterIDSiteRegistration
}

// GetName returns the display name of the importer.
func (importer *SiteRegistrationImporter) GetName() string {
	return "SiteRegistration"
}

func init() {
	registerImporter(&SiteRegistrationImporter{})
}
