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
	"html/template"
	"net/http"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/siteacc/config"
)

// Form represents the web interface form for user account registration.
type Form struct {
	conf *config.Configuration
	log  *zerolog.Logger

	tpl *template.Template
}

func (form *Form) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	form.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	form.log = log

	// Create the form template
	form.tpl = template.New("form")
	if _, err := form.tpl.Parse(formTemplate); err != nil {
		return errors.Wrap(err, "error while parsing form template")
	}

	return nil
}

// Execute generates the HTTP output of the form and writes it to the response writer.
func (form *Form) Execute(w http.ResponseWriter) error {
	type TemplateData struct {
	}

	tplData := TemplateData{}

	return form.tpl.Execute(w, tplData)
}

// NewForm creates a new web interface form.
func NewForm(conf *config.Configuration, log *zerolog.Logger) (*Form, error) {
	form := &Form{}
	if err := form.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the form")
	}
	return form, nil
}
