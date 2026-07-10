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

package notifications

import (
	"context"
	"encoding/json"
	"errors"

	oldnotification "github.com/cs3org/reva/v3/pkg/notification"
	oldhandler "github.com/cs3org/reva/v3/pkg/notification/handler"
	"github.com/cs3org/reva/v3/pkg/notification/handler/emailhandler"
	oldtemplate "github.com/cs3org/reva/v3/pkg/notification/template/registry"
	"github.com/mitchellh/mapstructure"
)

const EmailHandlerName = "email"

// EmailConfig configures the email notification handler.
type EmailConfig struct {
	Handler   map[string]any `mapstructure:"handler"`
	Templates map[string]any `mapstructure:"templates"`
}

// EmailHandler sends email notifications using the existing Reva email sender
// and template renderer.
type EmailHandler struct {
	templates oldtemplate.Registry
}

// NewEmailHandler creates an email handler from the same config shape used by
// the current notification system: SMTP settings plus template registrations.
func NewEmailHandler(ctx context.Context, m map[string]any) (*EmailHandler, error) {
	var conf EmailConfig
	if err := mapstructure.Decode(m, &conf); err != nil {
		return nil, err
	}

	sender, err := emailhandler.New(ctx, conf.Handler)
	if err != nil {
		return nil, err
	}

	templates := oldtemplate.New()
	handlers := map[string]oldhandler.Handler{
		EmailHandlerName: sender,
	}
	for _, raw := range conf.Templates {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		if _, err := templates.Put(data, handlers); err != nil {
			return nil, err
		}
	}

	return &EmailHandler{templates: *templates}, nil
}

// Name implements Handler.
func (h *EmailHandler) Name() string {
	return EmailHandlerName
}

// Send implements Handler.
func (h *EmailHandler) Send(_ context.Context, envelope Envelope) error {
	if envelope.TemplateName == "" {
		return errors.New("email notification requires a template name")
	}

	t, err := h.templates.Get(envelope.TemplateName)
	if err != nil {
		return err
	}

	n := oldnotification.Notification{
		TemplateName: envelope.TemplateName,
		Template:     *t,
		Ref:          envelope.DedupKey,
		Recipients:   envelope.Recipients,
	}
	return n.Send(envelope.Sender, envelope.TemplateData)
}
