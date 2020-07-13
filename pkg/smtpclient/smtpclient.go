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

package smtpclient

import (
	"bytes"
	"fmt"
	"net/smtp"

	"github.com/pkg/errors"
)

// SMTPCredentials stores the credentials required to connect to an SMTP server.
type SMTPCredentials struct {
	SenderMail     string `mapstructure:"sender_mail"`
	SenderPassword string `mapstructure:"sender_password"`
	SMTPServer     string `mapstructure:"smtp_server"`
	SMTPPort       int    `mapstructure:"smtp_port"`
	DisableAuth    bool   `mapstructure:"disable_auth"`
}

// SendMail allows sending mails using a set of client credentials.
func (creds *SMTPCredentials) SendMail(recipient, subject, body string) error {
	if creds.DisableAuth {
		return creds.sendMailSMTP(recipient, subject, body)
	}
	return creds.sendMailAuthSMTP(recipient, subject, body)
}

func (creds *SMTPCredentials) sendMailAuthSMTP(recipient, subject, body string) error {

	auth := smtp.PlainAuth("", creds.SenderMail, creds.SenderPassword, creds.SMTPServer)

	message := "From: " + creds.SenderMail + "\n" +
		"To: " + recipient + "\n" +
		"Subject: " + subject + "\n\n" +
		body

	err := smtp.SendMail(
		fmt.Sprintf("%s:%d", creds.SMTPServer, creds.SMTPPort),
		auth,
		creds.SenderMail,
		[]string{recipient},
		[]byte(message),
	)
	if err != nil {
		err = errors.Wrap(err, "smtpclient: error sending mail")
		return err
	}

	return nil
}

func (creds *SMTPCredentials) sendMailSMTP(recipient, subject, body string) error {

	c, err := smtp.Dial(fmt.Sprintf("%s:%d", creds.SMTPServer, creds.SMTPPort))
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Mail(creds.SenderMail); err != nil {
		return err
	}
	if err = c.Rcpt(recipient); err != nil {
		return err
	}

	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	message := "From: " + creds.SenderMail + "\n" +
		"To: " + recipient + "\n" +
		"Subject: " + subject + "\n\n" +
		body
	buf := bytes.NewBufferString(message)

	if _, err = buf.WriteTo(wc); err != nil {
		return err
	}

	return nil
}
