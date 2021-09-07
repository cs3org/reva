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

package account

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/siteacc/account/contact"
	"github.com/cs3org/reva/pkg/siteacc/account/edit"
	"github.com/cs3org/reva/pkg/siteacc/account/login"
	"github.com/cs3org/reva/pkg/siteacc/account/manage"
	"github.com/cs3org/reva/pkg/siteacc/account/registration"
	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Panel represents the account panel.
type Panel struct {
	html.PanelProvider

	conf *config.Configuration

	htmlPanel *html.Panel
}

const (
	templateLogin        = "login"
	templateManage       = "manage"
	templateEdit         = "edit"
	templateContact      = "contact"
	templateRegistration = "register"
)

func (panel *Panel) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	panel.conf = conf

	// Create the internal HTML panel
	htmlPanel, err := html.NewPanel("account-panel", panel, conf, log)
	if err != nil {
		return errors.Wrap(err, "unable to create the account panel")
	}
	panel.htmlPanel = htmlPanel

	// Add all templates
	if err := panel.htmlPanel.AddTemplate(templateLogin, &login.PanelTemplate{}); err != nil {
		return errors.Wrap(err, "unable to create the login template")
	}

	if err := panel.htmlPanel.AddTemplate(templateManage, &manage.PanelTemplate{}); err != nil {
		return errors.Wrap(err, "unable to create the account management template")
	}

	if err := panel.htmlPanel.AddTemplate(templateEdit, &edit.PanelTemplate{}); err != nil {
		return errors.Wrap(err, "unable to create the account editing template")
	}

	if err := panel.htmlPanel.AddTemplate(templateContact, &contact.PanelTemplate{}); err != nil {
		return errors.Wrap(err, "unable to create the contact template")
	}

	if err := panel.htmlPanel.AddTemplate(templateRegistration, &registration.PanelTemplate{}); err != nil {
		return errors.Wrap(err, "unable to create the registration template")
	}

	return nil
}

// GetActiveTemplate returns the name of the active template.
func (panel *Panel) GetActiveTemplate(session *html.Session, path string) string {
	validPaths := []string{templateLogin, templateManage, templateEdit, templateContact, templateRegistration}
	template := templateLogin

	// Only allow valid template paths; redirect to the login page otherwise
	for _, valid := range validPaths {
		if valid == path {
			template = path
			break
		}
	}

	return template
}

// PreExecute is called before the actual template is being executed.
func (panel *Panel) PreExecute(session *html.Session, path string, w http.ResponseWriter, r *http.Request) (html.ExecutionResult, error) {
	protectedPaths := []string{templateManage, templateEdit, templateContact}

	if session.LoggedInUser == nil {
		// If no user is logged in, redirect protected paths to the login page
		for _, protected := range protectedPaths {
			if protected == path {
				return panel.redirect(templateLogin, w, r), nil
			}
		}
	} else if path == templateLogin || path == templateRegistration {
		// If a user is logged in and tries to login or register again, redirect to the main account page
		return panel.redirect(templateManage, w, r), nil
	}

	return html.ContinueExecution, nil
}

// Execute generates the HTTP output of the form and writes it to the response writer.
func (panel *Panel) Execute(w http.ResponseWriter, r *http.Request, session *html.Session) error {
	dataProvider := func(*html.Session) interface{} {
		flatValues := make(map[string]string, len(r.URL.Query()))
		for k, v := range r.URL.Query() {
			flatValues[strings.Title(k)] = v[0]
		}

		availSites, err := data.QueryAvailableSites(panel.conf.Mentix.URL, panel.conf.Mentix.DataEndpoint)
		if err != nil {
			return errors.Wrap(err, "unable to query available sites")
		}

		type TemplateData struct {
			Account *data.Account
			Params  map[string]string

			Titles []string
			Sites  []data.SiteInformation
		}

		return TemplateData{
			Account: session.LoggedInUser,
			Params:  flatValues,
			Titles:  []string{"Mr", "Mrs", "Ms", "Prof", "Dr"},
			Sites:   availSites,
		}
	}
	return panel.htmlPanel.Execute(w, r, session, dataProvider)
}

func (panel *Panel) redirect(path string, w http.ResponseWriter, r *http.Request) html.ExecutionResult {
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

// NewPanel creates a new account panel.
func NewPanel(conf *config.Configuration, log *zerolog.Logger) (*Panel, error) {
	form := &Panel{}
	if err := form.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the account panel")
	}
	return form, nil
}
