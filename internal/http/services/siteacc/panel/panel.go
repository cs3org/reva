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

package panel

import (
	"html/template"
	"net/http"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/siteacc/config"
	"github.com/cs3org/reva/internal/http/services/siteacc/data"
)

// Panel represents the web interface panel of the accounts service.
type Panel struct {
	conf *config.Configuration
	log  *zerolog.Logger

	tpl *template.Template
}

func (panel *Panel) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	panel.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	panel.log = log

	// Create the panel template
	panel.tpl = template.New("panel")
	if _, err := panel.tpl.Parse(panelTemplate); err != nil {
		return errors.Wrap(err, "error while parsing panel template")
	}

	return nil
}

// Execute generates the HTTP output of the panel and writes it to the response writer.
func (panel *Panel) Execute(w http.ResponseWriter, accounts *data.Accounts) error {
	type TemplateData struct {
		Accounts *data.Accounts
	}

	tplData := TemplateData{
		Accounts: accounts,
	}

	return panel.tpl.Execute(w, tplData)
}

// NewPanel creates a new web interface panel.
func NewPanel(conf *config.Configuration, log *zerolog.Logger) (*Panel, error) {
	panel := &Panel{}
	if err := panel.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the panel")
	}
	return panel, nil
}
