// Copyright 2018-2022 CERN
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

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/cs3org/reva/pkg/siteacc/panels"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/contact"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/edit"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/login"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/manage"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/registration"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/settings"
	"github.com/cs3org/reva/pkg/siteacc/panels/account/sites"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Panel represents the account panel.
type Panel struct {
	panels.BasePanel
	html.PanelProvider
}

const (
	templateLogin        = "login"
	templateManage       = "manage"
	templateSettings     = "settings"
	templateEdit         = "edit"
	templateSites        = "sites"
	templateContact      = "contact"
	templateRegistration = "register"
)

func (panel *Panel) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	// Create templates
	templates := []panels.BasePanelTemplate{
		{
			ID:       templateLogin,
			Name:     "login",
			Provider: &login.PanelTemplate{},
		},
		{
			ID:       templateManage,
			Name:     "management",
			Provider: &manage.PanelTemplate{},
		},
		{
			ID:       templateSettings,
			Name:     "settings",
			Provider: &settings.PanelTemplate{},
		},
		{
			ID:       templateEdit,
			Name:     "editing",
			Provider: &edit.PanelTemplate{},
		},
		{
			ID:       templateSites,
			Name:     "sites",
			Provider: &sites.PanelTemplate{},
		},
		{
			ID:       templateContact,
			Name:     "contact",
			Provider: &contact.PanelTemplate{},
		},
		{
			ID:       templateRegistration,
			Name:     "registration",
			Provider: &registration.PanelTemplate{},
		},
	}

	// Initialize base
	if err := panel.BasePanel.Initialize("user-panel", panel, templates, conf, log); err != nil {
		return errors.Wrap(err, "unable to create the user panel")
	}

	return nil
}

// GetActiveTemplate returns the name of the active template.
func (panel *Panel) GetActiveTemplate(session *html.Session, path string) string {
	validPaths := []string{templateLogin, templateManage, templateSettings, templateEdit, templateSites, templateContact, templateRegistration}
	return panel.GetPathTemplate(validPaths, templateLogin, path)
}

// PreExecute is called before the actual template is being executed.
func (panel *Panel) PreExecute(session *html.Session, path string, w http.ResponseWriter, r *http.Request) (html.ExecutionResult, error) {
	protectedPaths := []string{templateManage, templateSettings, templateEdit, templateSites, templateContact}

	if user := session.LoggedInUser(); user != nil {
		switch path {
		case templateSites:
			// If the logged in user doesn't have sites access, redirect him back to the main account page
			if !user.Account.Data.SitesAccess {
				return panel.Redirect(templateManage, w, r), nil
			}

		case templateLogin:
		case templateRegistration:
			// If a user is logged in and tries to login or register again, redirect to the main account page
			return panel.Redirect(templateManage, w, r), nil
		}
	} else {
		// If no user is logged in, redirect protected paths to the login page
		for _, protected := range protectedPaths {
			if protected == path {
				return panel.Redirect(templateLogin, w, r), nil
			}
		}
	}

	return html.ContinueExecution, nil
}

// Execute generates the HTTP output of the panel and writes it to the response writer.
func (panel *Panel) Execute(w http.ResponseWriter, r *http.Request, session *html.Session) error {
	dataProvider := func(*html.Session) interface{} {
		flatValues := make(map[string]string, len(r.URL.Query()))
		for k, v := range r.URL.Query() {
			caser := cases.Title(language.Und)
			flatValues[caser.String(k)] = v[0]
		}

		availOps, err := data.QueryAvailableOperators(panel.Config().Mentix.URL, panel.Config().Mentix.DataEndpoint)
		if err != nil {
			return errors.Wrap(err, "unable to query available operators")
		}

		type TemplateData struct {
			Operator *data.Operator
			Account  *data.Account
			Params   map[string]string

			Operators []data.OperatorInformation
			Sites     map[string]string
			Titles    []string
		}

		tplData := TemplateData{
			Operator:  nil,
			Account:   nil,
			Params:    flatValues,
			Operators: availOps,
			Sites:     make(map[string]string, 10),
			Titles:    []string{"Mr", "Mrs", "Ms", "Prof", "Dr"},
		}
		if user := session.LoggedInUser(); user != nil {
			availSites, err := panel.FetchOperatorSites(user.Operator)
			if err != nil {
				return errors.Wrap(err, "unable to query available sites")
			}

			tplData.Operator = panel.CloneOperator(user.Operator, availSites)
			tplData.Account = user.Account
			tplData.Sites = availSites
		}
		return tplData
	}
	return panel.BasePanel.Execute(w, r, session, dataProvider)
}

// NewPanel creates a new account panel.
func NewPanel(conf *config.Configuration, log *zerolog.Logger) (*Panel, error) {
	form := &Panel{}
	if err := form.initialize(conf, log); err != nil {
		return nil, errors.Wrap(err, "unable to initialize the account panel")
	}
	return form, nil
}
