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

package registration

import (
	"net/http"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Form represents the web interface form for user account registration.
type Form struct {
	html.ContentProvider

	htmlPanel *html.Panel
}

func (form *Form) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	// Create the internal HTML panel
	htmlPanel, err := html.NewPanel("registration-form", form, conf, log)
	if err != nil {
		return errors.Wrap(err, "unable to create the registration form")
	}
	form.htmlPanel = htmlPanel

	return nil
}

// GetTitle returns the title of the htmlPanel.
func (form *Form) GetTitle() string {
	return "ScienceMesh Account Registration"
}

// GetCaption returns the caption which is displayed on the htmlPanel.
func (form *Form) GetCaption() string {
	return "Welcome to the ScienceMesh Account Registration!"
}

// GetContentJavaScript delivers additional JavaScript code.
func (form *Form) GetContentJavaScript() string {
	return tplJavaScript
}

// GetContentStyleSheet delivers additional stylesheet code.
func (form *Form) GetContentStyleSheet() string {
	return tplStyleSheet
}

// GetContentBody delivers the actual body content.
func (form *Form) GetContentBody() string {
	return tplBody
}

// Execute generates the HTTP output of the form and writes it to the response writer.
func (form *Form) Execute(w http.ResponseWriter) error {
	type TemplateData struct {
	}

	tplData := TemplateData{}

	return form.htmlPanel.Execute(w, tplData)
}

// NewForm creates a new web interface form.
func NewForm(conf *config.Configuration, log *zerolog.Logger) (*Form, error) {
	form := &Form{}
	if err := form.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the registration form")
	}
	return form, nil
}
