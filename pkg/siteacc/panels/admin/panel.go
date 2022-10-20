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

package admin

import (
	"net/http"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/cs3org/reva/pkg/siteacc/panels"
	"github.com/cs3org/reva/pkg/siteacc/panels/admin/accounts"
	"github.com/cs3org/reva/pkg/siteacc/panels/admin/manage"
	"github.com/cs3org/reva/pkg/siteacc/panels/admin/sites"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Panel represents the web interface panel of the accounts service administration.
type Panel struct {
	panels.BasePanel
	html.PanelProvider
}

const (
	templateManage   = "manage"
	templateAccounts = "accounts"
	templateSites    = "sites"
)

func (panel *Panel) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	// Create templates
	templates := []panels.BasePanelTemplate{
		{
			ID:       templateManage,
			Name:     "mangement",
			Provider: &manage.PanelTemplate{},
		},
		{
			ID:       templateAccounts,
			Name:     "accounts",
			Provider: &accounts.PanelTemplate{},
		},
		{
			ID:       templateSites,
			Name:     "sites",
			Provider: &sites.PanelTemplate{},
		},
	}

	// Initialize base
	if err := panel.BasePanel.Initialize("admin-panel", panel, templates, conf, log); err != nil {
		return errors.Wrap(err, "unable to create the administrator panel")
	}

	return nil
}

// GetActiveTemplate returns the name of the active template.
func (panel *Panel) GetActiveTemplate(session *html.Session, path string) string {
	validPaths := []string{templateManage, templateAccounts, templateSites}
	return panel.GetPathTemplate(validPaths, templateManage, path)
}

// PreExecute is called before the actual template is being executed.
func (panel *Panel) PreExecute(*html.Session, string, http.ResponseWriter, *http.Request) (html.ExecutionResult, error) {
	return html.ContinueExecution, nil
}

// Execute generates the HTTP output of the panel and writes it to the response writer.
func (panel *Panel) Execute(w http.ResponseWriter, r *http.Request, session *html.Session, accounts *data.Accounts, operators *data.Operators) error {
	// Clone all operators
	opsClone, err := panel.cloneOperators(operators)
	if err != nil {
		return errors.Wrap(err, "unable to clone operators")
	}

	dataProvider := func(*html.Session) interface{} {
		type TemplateData struct {
			Accounts  *data.Accounts
			Operators *data.Operators
		}

		return TemplateData{
			Accounts:  accounts,
			Operators: opsClone,
		}
	}
	return panel.BasePanel.Execute(w, r, session, dataProvider)
}

func (panel *Panel) cloneOperators(operators *data.Operators) (*data.Operators, error) {
	// Clone all available operators and decrypt all credentials for the panel
	opsClone := make(data.Operators, 0, len(*operators))
	for _, op := range *operators {
		availSites, err := panel.FetchOperatorSites(op)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to query available sites of operator %v", op.ID)
		}
		opsClone = append(opsClone, panel.CloneOperator(op, availSites))
	}
	return &opsClone, nil
}

// NewPanel creates a new administration panel.
func NewPanel(conf *config.Configuration, log *zerolog.Logger) (*Panel, error) {
	panel := &Panel{}
	if err := panel.initialize(conf, log); err != nil {
		return nil, errors.Wrap(err, "unable to initialize the administration panel")
	}
	return panel, nil
}
