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

package email

import (
	"bytes"
	"text/template"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/internal/http/services/siteacc/data"
	"github.com/cs3org/reva/pkg/smtpclient"
)

// SendAccountCreated sends an email about account creation.
func SendAccountCreated(account *data.Account, recipients []string, smtp *smtpclient.SMTPCredentials) error {
	return send(recipients, "ScienceMesh: Site account created", accountCreatedTemplate, account, smtp)
}

// SendAPIKeyAssigned sends an email about API key assignment.
func SendAPIKeyAssigned(account *data.Account, recipients []string, smtp *smtpclient.SMTPCredentials) error {
	return send(recipients, "ScienceMesh: Your API key", apiKeyAssignedTemplate, account, smtp)
}

// SendAccountAuthorized sends an email about account authorization.
func SendAccountAuthorized(account *data.Account, recipients []string, smtp *smtpclient.SMTPCredentials) error {
	return send(recipients, "ScienceMesh: Site registration authorized", accountAuthorizedTemplate, account, smtp)
}

func send(recipients []string, subject string, bodyTemplate string, data interface{}, smtp *smtpclient.SMTPCredentials) error {
	// Do not fail if no SMTP client or recipient is given
	if smtp == nil {
		return nil
	}

	tpl := template.New("email")
	if _, err := tpl.Parse(bodyTemplate); err != nil {
		return errors.Wrap(err, "error while parsing email template")
	}

	var body bytes.Buffer
	if err := tpl.Execute(&body, data); err != nil {
		return errors.Wrap(err, "error while executing email template")
	}

	for _, recipient := range recipients {
		if len(recipient) == 0 {
			continue
		}

		// Send the mail w/o blocking the main thread
		go func(recipient string) {
			_ = smtp.SendMail(recipient, subject, body.String())
		}(recipient)
	}

	return nil
}
