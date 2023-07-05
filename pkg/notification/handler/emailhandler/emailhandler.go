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

package emailhandler

import (
	"context"
	"fmt"
	"net/smtp"
	"regexp"
	"strings"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/notification/handler"
	"github.com/cs3org/reva/pkg/notification/handler/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

func init() {
	registry.Register("email", New)
}

// EmailHandler is the notification handler for emails.
type EmailHandler struct {
	conf *config
	log  *zerolog.Logger
}

type config struct {
	SMTPAddress    string `mapstructure:"smtp_server" docs:";The hostname and port of the SMTP server."`
	SenderLogin    string `mapstructure:"sender_login" docs:";The email to be used to send mails."`
	SenderPassword string `mapstructure:"sender_password" docs:";The sender's password."`
	DisableAuth    bool   `mapstructure:"disable_auth" docs:"false;Whether to disable SMTP auth."`
	DefaultSender  string `mapstructure:"default_sender" docs:"no-reply@cernbox.cern.ch;Default sender when not specified in the trigger."`
}

func (c *config) ApplyDefaults() {
	if c.DefaultSender == "" {
		c.DefaultSender = "no-reply@cernbox.cern.ch"
	}
}

// New returns a new email handler.
func New(ctx context.Context, m map[string]any) (handler.Handler, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	return &EmailHandler{
		conf: &c,
		log:  log,
	}, nil
}

// Send is the method run when a notification is triggered for this handler.
func (e *EmailHandler) Send(sender, recipient, subject, body string) error {
	if sender == "" {
		sender = e.conf.DefaultSender
	}

	msg := e.generateMsg(sender, recipient, subject, body)
	err := smtp.SendMail(e.conf.SMTPAddress, e.getAuth(), sender, []string{recipient}, msg)
	if err != nil {
		return err
	}

	e.log.Debug().Msgf("mail sent to recipient %s", recipient)

	return nil
}

func (e *EmailHandler) getAuth() smtp.Auth {
	if e.conf.DisableAuth {
		return nil
	}

	return smtp.PlainAuth("", e.conf.SenderLogin, e.conf.SenderPassword, strings.SplitN(e.conf.SMTPAddress, ":", 2)[0])
}

func (e *EmailHandler) generateMsg(from, to, subject, body string) []byte {
	re := regexp.MustCompile(`\r?\n`)
	cleanSubject := re.ReplaceAllString(strings.TrimSpace(subject), " ")
	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", cleanSubject),
		"MIME-version: 1.0;",
		"Content-Type: text/html; charset=\"UTF-8\";",
	}

	var sb strings.Builder

	for _, h := range headers {
		sb.WriteString(h)
		sb.WriteString("\r\n")
	}
	sb.WriteString("\r\n")
	sb.WriteString(body)

	return []byte(sb.String())
}
