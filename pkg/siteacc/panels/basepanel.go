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

package panels

import (
	"net/http"
	"strings"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// BasePanel represents the base for all panels.
type BasePanel struct {
	conf *config.Configuration

	htmlPanel *html.Panel
}

type BasePanelTemplate struct {
	ID       string
	Name     string
	Provider html.ContentProvider
}

// Initialize initializes the base panel.
func (panel *BasePanel) Initialize(name string, panelProvider html.PanelProvider, templates []BasePanelTemplate, conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	panel.conf = conf

	// Create the internal HTML panel
	htmlPanel, err := html.NewPanel(name, panelProvider, conf, log)
	if err != nil {
		return errors.Wrap(err, "unable to create the HTML panel")
	}
	panel.htmlPanel = htmlPanel

	// Add all templates
	for _, template := range templates {
		if err := panel.htmlPanel.AddTemplate(template.ID, template.Provider); err != nil {
			return errors.Wrapf(err, "unable to create the %v template", template.Name)
		}
	}

	return nil
}

// GetPathTemplate returns the name of the active template.
func (panel *BasePanel) GetPathTemplate(validPaths []string, defaultTemplate string, path string) string {
	template := defaultTemplate

	// Only allow valid template paths; redirect to the default template otherwise
	for _, valid := range validPaths {
		if valid == path {
			template = path
			break
		}
	}

	return template
}

func (panel *BasePanel) Execute(w http.ResponseWriter, r *http.Request, session *html.Session, dataProvider html.PanelDataProvider) error {
	return panel.htmlPanel.Execute(w, r, session, dataProvider)
}

// CloneOperator clones an operator and adds missing sites.
func (panel *BasePanel) CloneOperator(op *data.Operator, sites map[string]string) *data.Operator {
	// Clone the operator and decrypt all credentials for the panel
	opClone := op.Clone(false)
	for _, site := range opClone.Sites {
		id, secret, err := site.Config.TestClientCredentials.Get(panel.conf.Security.CredentialsPassphrase)
		if err == nil {
			site.Config.TestClientCredentials.ID = id
			site.Config.TestClientCredentials.Secret = secret
		}
	}

	// Add missing sites
	for id := range sites {
		siteFound := false
		for _, site := range opClone.Sites {
			if strings.EqualFold(site.ID, id) {
				siteFound = true
				break
			}
		}
		if !siteFound {
			opClone.Sites = append(opClone.Sites, &data.Site{
				ID:     id,
				Config: data.SiteConfiguration{},
			})
		}
	}

	return opClone
}

func (panel *BasePanel) Config() *config.Configuration {
	return panel.conf
}
