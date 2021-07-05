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

package html

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Panel provides basic HTML panel functionality.
type Panel struct {
	conf *config.Configuration
	log  *zerolog.Logger

	provider ContentProvider

	tpl *template.Template
}

func (panel *Panel) initialize(name string, provider ContentProvider, conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	panel.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	panel.log = log

	if provider == nil {
		return errors.Errorf("no content provider provided")
	}
	panel.provider = provider

	// Create the panel template
	content, err := panel.compile()
	if err != nil {
		return errors.Wrap(err, "error while compiling the panel template")
	}

	panel.tpl = template.New(name)
	if _, err := panel.tpl.Parse(content); err != nil {
		return errors.Wrap(err, "error while parsing the panel template")
	}

	return nil
}

func (panel *Panel) compile() (string, error) {
	content := panelTemplate

	// Replace placeholders by the values provided by the content provider
	content = strings.ReplaceAll(content, "$(TITLE)", panel.provider.GetTitle())
	content = strings.ReplaceAll(content, "$(CAPTION)", panel.provider.GetCaption())

	content = strings.ReplaceAll(content, "$(CONTENT_JAVASCRIPT)", panel.provider.GetContentJavaScript())
	content = strings.ReplaceAll(content, "$(CONTENT_STYLESHEET)", panel.provider.GetContentStyleSheet())
	content = strings.ReplaceAll(content, "$(CONTENT_BODY)", panel.provider.GetContentBody())

	return content, nil
}

// Execute generates the HTTP output of the panel and writes it to the response writer.
func (panel *Panel) Execute(w http.ResponseWriter, data interface{}) error {
	return panel.tpl.Execute(w, data)
}

// NewPanel creates a new panel.
func NewPanel(name string, provider ContentProvider, conf *config.Configuration, log *zerolog.Logger) (*Panel, error) {
	panel := &Panel{}
	if err := panel.initialize(name, provider, conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the panel")
	}
	return panel, nil
}
