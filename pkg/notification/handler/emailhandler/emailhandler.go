// Copyright 2018-2024 CERN
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
	"os"
	"path"
	"strconv"
	"strings"

	stdmail "net/mail"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notification/handler"
	"github.com/cs3org/reva/v3/pkg/notification/handler/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"gopkg.in/mail.v2"
)

func init() {
	registry.Register("email", New)
}

// EmailHandler is the notification handler for emails.
type EmailHandler struct {
	conf *config
	log  *zerolog.Logger
	// Paths of CID files to be embedded, relative to the CIDFolder
	// so in principle, should always be just the filename
	CIDRelPaths []string
	CIDFolder   string
}

type config struct {
	SMTPAddress    string `docs:";The hostname and port of the SMTP server."                                 mapstructure:"smtp_server"`
	SenderLogin    string `docs:";The email to be used to send mails."                                       mapstructure:"sender_login"`
	SenderPassword string `docs:";The sender's password."                                                    mapstructure:"sender_password"`
	DisableAuth    bool   `docs:"false;Whether to disable SMTP auth."                                        mapstructure:"disable_auth"`
	DefaultSender  string `docs:"no-reply@cernbox.cern.ch;Default sender when not specified in the trigger." mapstructure:"default_sender"`
	CIDFolder      string `docs:"/etc/revad/cid/;Folder on the local filesystem that includes files to be embedded as CIDs in emails." mapstructure:"cid_folder"`
}

func (c *config) ApplyDefaults() {
	if c.DefaultSender == "" {
		c.DefaultSender = "no-reply@cernbox.cern.ch"
	}
	if c.CIDFolder == "" {
		c.CIDFolder = "/etc/revad/cid/"
	}
}

// New returns a new email handler.
func New(ctx context.Context, m map[string]any) (handler.Handler, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	handler := &EmailHandler{
		conf: &c,
		log:  log,
	}

	if len(strings.Split(c.SMTPAddress, ":")) != 2 {
		return nil, fmt.Errorf("Invalid SMTP address: %s, must be in the format host:port", c.SMTPAddress)
	}

	// Find CID files that we can embed in emails
	handler.CIDFolder = c.CIDFolder
	files, err := listFilenames(c.CIDFolder)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to list files in CID folder %s, continuing without embedding anything!", c.CIDFolder)
	} else {
		log.Info().Msgf("Registered %d files from the CID folder with the email handler", len(files))
		handler.CIDRelPaths = files
	}
	return handler, nil

}

// Find the files in the given CID Folder
func listFilenames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.Name())
	}
	return files, nil
}

// SendEmail, obviously, sends an email.
// CIDPaths are files that are to be embedded in the email.
// They can be referenced in the body using e.g. `<img src="cid:file.png">`
// Note that paths specified here must be relative to the asset directory!
func (e *EmailHandler) Send(sender, recipient, subject, body string) error {
	if strings.TrimSpace(sender) == "" {
		sender = e.conf.DefaultSender
	}
	if strings.TrimSpace(recipient) == "" {
		return errors.New("recipient address cannot be empty")
	}

	// Validate email address format
	if _, err := stdmail.ParseAddress(sender); err != nil {
		sender = e.conf.DefaultSender
	}
	if _, err := stdmail.ParseAddress(recipient); err != nil {
		return errors.Wrapf(err, "invalid recipient address format: %s", recipient)
	}

	message := mail.NewMessage()
	message.SetHeader("From", sender)
	message.SetHeader("To", recipient)
	message.SetHeader("Subject", subject)

	// Embed CID files with error handling
	for _, cid := range e.CIDRelPaths {
		if strings.Contains(body, cid) {
			fullPath := path.Join(e.CIDFolder, cid)
			message.Embed(fullPath)
		}
	}

	message.SetBody("text/html", body)

	// Parse SMTP address
	splitAddress := strings.SplitN(e.conf.SMTPAddress, ":", 2)
	if len(splitAddress) != 2 {
		return errors.New("invalid SMTP address format: expected host:port")
	}

	host, portStr := splitAddress[0], splitAddress[1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errors.Wrapf(err, "invalid SMTP port: %s", portStr)
	}

	var d *mail.Dialer
	if e.conf.DisableAuth {
		d = &mail.Dialer{
			Host: host,
			Port: port,
		}
	} else {
		d = &mail.Dialer{
			Host:     host,
			Port:     port,
			Username: e.conf.SenderLogin,
			Password: e.conf.SenderPassword,
		}
	}

	return d.DialAndSend(message)
}
