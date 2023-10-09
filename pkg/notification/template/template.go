// Copyright 2018-2023 CERN
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

package template

import (
	"bytes"
	"errors"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	textTemplate "text/template"

	"github.com/cs3org/reva/pkg/notification/handler"
	"github.com/mitchellh/mapstructure"
)

const validTemplateNameRegex = "[a-zA-Z0-9-]"

// RegistrationRequest represents a Template registration request.
type RegistrationRequest struct {
	Name            string `json:"name"                  mapstructure:"name"`
	Handler         string `json:"handler"               mapstructure:"handler"`
	BodyTmplPath    string `json:"body_template_path"    mapstructure:"body_template_path"`
	SubjectTmplPath string `json:"subject_template_path" mapstructure:"subject_template_path"`
	Persistent      bool   `json:"persistent"            mapstructure:"persistent"`
}

// Template represents a notification template.
type Template struct {
	Name        string
	Handler     handler.Handler
	Persistent  bool
	tmplSubject *textTemplate.Template
	tmplBody    *htmlTemplate.Template
}

// FileNotFoundError is the error returned when a template file is missing.
type FileNotFoundError struct {
	TemplateFileName string
	Err              error
}

// Error returns the string error msg for FileNotFoundError.
func (t FileNotFoundError) Error() string {
	return fmt.Sprintf("template file %s not found", t.TemplateFileName)
}

// New creates a new Template from a RegistrationRequest.
func New(m map[string]interface{}, hs map[string]handler.Handler) (*Template, string, error) {
	rr := &RegistrationRequest{}
	if err := mapstructure.Decode(m, rr); err != nil {
		return nil, rr.Name, err
	}

	h, ok := hs[rr.Handler]
	if !ok {
		return nil, rr.Name, fmt.Errorf("unknown handler %s", rr.Handler)
	}

	tmplSubject, err := parseTmplFile(rr.SubjectTmplPath, "subject")
	if err != nil {
		return nil, rr.Name, err
	}

	tmplBody, err := parseTmplFile(rr.BodyTmplPath, "body")
	if err != nil {
		return nil, rr.Name, err
	}

	t := &Template{
		Name:        rr.Name,
		Handler:     h,
		tmplSubject: tmplSubject.(*textTemplate.Template),
		tmplBody:    tmplBody.(*htmlTemplate.Template),
	}

	if err := CheckTemplateName(t.Name); err != nil {
		return nil, rr.Name, err
	}

	return t, rr.Name, nil
}

// RenderSubject renders the subject template.
func (t *Template) RenderSubject(arguments map[string]interface{}) (string, error) {
	var buf bytes.Buffer
	err := t.tmplSubject.Execute(&buf, arguments)
	return buf.String(), err
}

// RenderBody renders the body template.
func (t *Template) RenderBody(arguments map[string]interface{}) (string, error) {
	var buf bytes.Buffer
	err := t.tmplBody.Execute(&buf, arguments)
	return buf.String(), err
}

// CheckTemplateName validates the name of the template.
func CheckTemplateName(name string) error {
	if name == "" {
		return errors.New("template name cannot be empty")
	}

	re := regexp.MustCompile(validTemplateNameRegex)
	invalidChars := re.ReplaceAllString(name, "")
	if len(invalidChars) > 0 {
		return fmt.Errorf("template name %s must contain only %s", name, validTemplateNameRegex)
	}

	return nil
}

func parseTmplFile(path, name string) (interface{}, error) {
	if path == "" {
		return textTemplate.New(name).Parse("")
	}

	ext := filepath.Ext(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, &FileNotFoundError{
			TemplateFileName: path,
			Err:              err,
		}
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	switch ext {
	case ".txt":
		tmpl, err := textTemplate.New(name).Parse(string(data))
		if err != nil {
			return nil, err
		}

		return tmpl, nil
	case ".html":
		tmpl, err := htmlTemplate.New(name).Parse(string(data))
		if err != nil {
			return nil, err
		}

		return tmpl, nil
	default:
		return nil, errors.New("unknown template type")
	}
}
