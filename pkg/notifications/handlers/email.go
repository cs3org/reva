// Copyright 2018-2026 CERN
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

package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmlTemplate "html/template"
	"io"
	stdmail "net/mail"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	textTemplate "text/template"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	"gopkg.in/mail.v2"
)

const EmailHandlerName = "email"

const validTemplateNameRegex = "[a-zA-Z0-9-]"

// EmailConfig configures the email notification handler.
type EmailConfig struct {
	SMTPAddress    string         `docs:";The hostname and port of the SMTP server." mapstructure:"smtp_server"`
	SenderLogin    string         `docs:";The email to be used to send mails."       mapstructure:"sender_login"`
	SenderPassword string         `docs:";The sender's password."                    mapstructure:"sender_password"`
	DisableAuth    bool           `docs:"false;Whether to disable SMTP auth."        mapstructure:"disable_auth"`
	DefaultSender  string         `docs:"no-reply@cernbox.cern.ch;Default sender when not specified in the request." mapstructure:"default_sender"`
	CIDFolder      string         `docs:"/etc/revad/cid/;Folder containing files to embed as CIDs in emails." mapstructure:"cid_folder"`
	Templates      map[string]any `docs:"nil;Email notification templates." mapstructure:"templates"`
}

func (c *EmailConfig) ApplyDefaults() {
	if c.DefaultSender == "" {
		c.DefaultSender = "no-reply@cernbox.cern.ch"
	}
	if c.CIDFolder == "" {
		c.CIDFolder = "/etc/revad/cid/"
	}
}

type emailTemplateRegistration struct {
	Name            string `json:"name"                  mapstructure:"name"`
	BodyTmplPath    string `json:"body_template_path"    mapstructure:"body_template_path"`
	SubjectTmplPath string `json:"subject_template_path" mapstructure:"subject_template_path"`
}

type emailTemplate struct {
	name        string
	tmplSubject *textTemplate.Template
	tmplBody    *htmlTemplate.Template
}

// EmailHandler sends email notifications using configured templates.
type EmailHandler struct {
	conf        *EmailConfig
	log         *zerolog.Logger
	cidRelPaths []string
	cidFolder   string
	templates   map[string]*emailTemplate
}

// NewEmailHandler creates an email handler from SMTP settings and template registrations.
func NewEmailHandler(ctx context.Context, m map[string]any) (*EmailHandler, error) {
	var conf EmailConfig
	if err := cfg.Decode(m, &conf); err != nil {
		return nil, err
	}
	conf.ApplyDefaults()

	if len(strings.Split(conf.SMTPAddress, ":")) != 2 {
		return nil, fmt.Errorf("invalid SMTP address %q, must be in the format host:port", conf.SMTPAddress)
	}

	log := appctx.GetLogger(ctx)
	h := &EmailHandler{
		conf:      &conf,
		log:       log,
		cidFolder: conf.CIDFolder,
		templates: make(map[string]*emailTemplate, len(conf.Templates)),
	}

	files, err := listFilenames(conf.CIDFolder)
	if err != nil {
		log.Error().Err(err).Msgf("failed to list files in CID folder %s, continuing without embedded CIDs", conf.CIDFolder)
	} else {
		log.Info().Msgf("registered %d files from the CID folder with the email handler", len(files))
		h.cidRelPaths = files
	}

	for name, raw := range conf.Templates {
		t, err := newEmailTemplate(raw)
		if err != nil {
			return nil, fmt.Errorf("registering email template %q failed: %w", name, err)
		}
		h.templates[t.name] = t
	}

	return h, nil
}

// Name implements Handler.
func (h *EmailHandler) Name() string {
	return EmailHandlerName
}

// Send implements Handler.
func (h *EmailHandler) Send(_ context.Context, envelope model.Envelope) error {
	log := h.log.With().
		Str("notification_id", envelope.ID).
		Str("event_type", envelope.EventType).
		Str("template_name", envelope.TemplateName).
		Int("recipients", len(envelope.Recipients)).
		Logger()

	if envelope.TemplateName == "" {
		err := errors.New("email notification requires a template name")
		log.Error().Err(err).Msg("notifications: email handler failed")
		return err
	}
	t, ok := h.templates[envelope.TemplateName]
	if !ok {
		err := fmt.Errorf("email template %s not found", envelope.TemplateName)
		log.Error().Err(err).Msg("notifications: email handler failed")
		return err
	}

	subject, err := t.renderSubject(envelope.TemplateData)
	if err != nil {
		log.Error().Err(err).Msg("notifications: email handler failed to render subject")
		return err
	}
	body, err := t.renderBody(envelope.TemplateData)
	if err != nil {
		log.Error().Err(err).Msg("notifications: email handler failed to render body")
		return err
	}

	for _, recipient := range envelope.Recipients {
		recipientLog := log.With().Str("recipient", recipient).Logger()
		recipientLog.Info().Msg("notifications: email handler sending email")
		if err := h.sendEmail(envelope.Sender, recipient, subject, body); err != nil {
			recipientLog.Error().Err(err).Msg("notifications: email handler failed to send email")
			return err
		}
		recipientLog.Info().Msg("notifications: email handler sent email")
	}
	return nil
}

func newEmailTemplate(raw any) (*emailTemplate, error) {
	var rr emailTemplateRegistration
	if err := mapstructure.Decode(raw, &rr); err != nil {
		return nil, err
	}
	if err := checkTemplateName(rr.Name); err != nil {
		return nil, err
	}

	tmplSubject, err := parseTmplFile(rr.SubjectTmplPath, "subject")
	if err != nil {
		return nil, err
	}
	tmplBody, err := parseTmplFile(rr.BodyTmplPath, "body")
	if err != nil {
		return nil, err
	}

	return &emailTemplate{
		name:        rr.Name,
		tmplSubject: tmplSubject.(*textTemplate.Template),
		tmplBody:    tmplBody.(*htmlTemplate.Template),
	}, nil
}

func (t *emailTemplate) renderSubject(arguments map[string]any) (string, error) {
	var buf bytes.Buffer
	err := t.tmplSubject.Execute(&buf, arguments)
	return buf.String(), err
}

func (t *emailTemplate) renderBody(arguments map[string]any) (string, error) {
	var buf bytes.Buffer
	err := t.tmplBody.Execute(&buf, arguments)
	return buf.String(), err
}

func (h *EmailHandler) sendEmail(sender, recipient, subject, body string) error {
	if strings.TrimSpace(sender) == "" {
		sender = h.conf.DefaultSender
	}
	if strings.TrimSpace(recipient) == "" {
		return errors.New("recipient address cannot be empty")
	}

	if _, err := stdmail.ParseAddress(sender); err != nil {
		sender = h.conf.DefaultSender
	}
	if _, err := stdmail.ParseAddress(recipient); err != nil {
		return fmt.Errorf("invalid recipient address format %s: %w", recipient, err)
	}

	message := mail.NewMessage()
	message.SetHeader("From", sender)
	message.SetHeader("To", recipient)
	message.SetHeader("Subject", subject)

	for _, cid := range h.cidRelPaths {
		if strings.Contains(body, cid) {
			message.Embed(path.Join(h.cidFolder, cid))
		}
	}

	message.SetBody("text/html", body)

	host, portStr, ok := strings.Cut(h.conf.SMTPAddress, ":")
	if !ok {
		return errors.New("invalid SMTP address format: expected host:port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid SMTP port %s: %w", portStr, err)
	}

	dialer := &mail.Dialer{
		Host: host,
		Port: port,
	}
	if !h.conf.DisableAuth {
		dialer.Username = h.conf.SenderLogin
		dialer.Password = h.conf.SenderPassword
	}

	return dialer.DialAndSend(message)
}

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

func checkTemplateName(name string) error {
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

func parseTmplFile(path, name string) (any, error) {
	if path == "" {
		return textTemplate.New(name).Parse("")
	}

	funcMap := map[string]any{
		"split":         strings.Split,
		"join":          strings.Join,
		"trim":          strings.TrimSpace,
		"contains":      strings.Contains,
		"replace":       strings.Replace,
		"base":          filepath.Base,
		"dir":           filepath.Dir,
		"clean":         filepath.Clean,
		"ext":           filepath.Ext,
		"atoi":          strconv.Atoi,
		"atoi64":        strconv.ParseInt,
		"ftoa":          strconv.FormatFloat,
		"now":           time.Now,
		"list":          func(items ...string) []string { return items },
		"sliceContains": func(slice []string, s string) bool { return slices.Contains(slice, s) },
		"css":           func(s string) htmlTemplate.CSS { return htmlTemplate.CSS(s) },
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("template file %s not found: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	switch filepath.Ext(path) {
	case ".txt":
		return textTemplate.New(name).Funcs(funcMap).Parse(string(data))
	case ".html":
		return htmlTemplate.New(name).Funcs(funcMap).Parse(string(data))
	default:
		return nil, errors.New("unknown template type")
	}
}
