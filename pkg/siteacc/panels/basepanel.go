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
	"net/url"
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

// BasePanelTemplate represents an HTML template used for initialization.
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

// Execute generates the HTTP output of the panel and writes it to the response writer.
func (panel *BasePanel) Execute(w http.ResponseWriter, r *http.Request, session *html.Session, dataProvider html.PanelDataProvider) error {
	return panel.htmlPanel.Execute(w, r, session, dataProvider)
}

// Redirect performs an HTTP redirect.
func (panel *BasePanel) Redirect(path string, w http.ResponseWriter, r *http.Request) html.ExecutionResult {
	// Check if the original (full) URI path is stored in the request header; if not, use the request URI to get the path
	fullPath := r.Header.Get("X-Replaced-Path")
	if fullPath == "" {
		uri, _ := url.Parse(r.RequestURI)
		fullPath = uri.Path
	}

	// Modify the original request URL by replacing the path parameter
	newURL, _ := url.Parse(fullPath)
	params := newURL.Query()
	params.Del("path")
	params.Add("path", path)
	newURL.RawQuery = params.Encode()
	http.Redirect(w, r, newURL.String(), http.StatusFound)
	return html.AbortExecution
}

// FetchOperatorSites fetches all sites for an operator using Mentix.
func (panel *BasePanel) FetchOperatorSites(op *data.Operator) (map[string]string, error) {
	ids, err := data.QueryOperatorSites(op.ID, panel.Config().Mentix.URL, panel.Config().Mentix.DataEndpoint)
	if err != nil {
		return nil, err
	}
	sites := make(map[string]string, 10)
	for _, id := range ids {
		if siteName, _ := data.QuerySiteName(id, true, panel.Config().Mentix.URL, panel.Config().Mentix.DataEndpoint); err == nil {
			sites[id] = siteName
		} else {
			sites[id] = id
		}
	}
	return sites, nil
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

// Config gets the configuration object.
func (panel *BasePanel) Config() *config.Configuration {
	return panel.conf
}
