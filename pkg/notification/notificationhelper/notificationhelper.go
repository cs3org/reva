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

package notificationhelper

import (
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/pkg/notification"
	"github.com/cs3org/reva/pkg/notification/template"
	"github.com/cs3org/reva/pkg/notification/trigger"
	"github.com/cs3org/reva/pkg/notification/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// NotificationHelper is the type used in services to work with notifications.
type NotificationHelper struct {
	Name string
	Conf *Config
	Log  *zerolog.Logger
	nc   *nats.Conn
	js   nats.JetStreamContext
	kv   nats.KeyValue
}

// Config contains the configuration for the Notification Helper.
type Config struct {
	NatsAddress string                 `mapstructure:"nats_address" docs:";The NATS server address."`
	NatsToken   string                 `mapstructure:"nats_token" docs:";The token to authenticate against the NATS server"`
	NatsStream  string                 `mapstructure:"nats_stream" docs:"reva-notifications;The notifications NATS stream."`
	Templates   map[string]interface{} `mapstructure:"templates" docs:";Notification templates for the service."`
}

func defaultConfig() *Config {
	return &Config{
		NatsStream: "reva-notifications",
	}
}

// New creates a new Notification Helper.
func New(name string, m map[string]interface{}, log *zerolog.Logger) *NotificationHelper {
	annotatedLogger := log.With().Str("service", name).Str("scope", "notifications").Logger()

	conf := defaultConfig()
	nh := &NotificationHelper{
		Name: name,
		Conf: conf,
		Log:  &annotatedLogger,
	}

	if len(m) == 0 {
		log.Info().Msgf("no 'notifications' field in service config, notifications will be disabled")
		return nh
	}

	if err := mapstructure.Decode(m, conf); err != nil {
		log.Error().Err(err).Msgf("decoding config failed, notifications will be disabled")
		return nh
	}

	if err := nh.connect(); err != nil {
		log.Error().Err(err).Msgf("connecting to nats failed, notifications will be disabled")
		return nh
	}

	nh.registerTemplates(nh.Conf.Templates)

	return nh
}

func (nh *NotificationHelper) connect() error {
	nc, err := utils.ConnectToNats(nh.Conf.NatsAddress, nh.Conf.NatsToken, *nh.Log)
	if err != nil {
		return err
	}
	nh.nc = nc

	js, err := nh.nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return errors.Wrap(err, "jetstream initialization failed")
	}
	stream, _ := js.StreamInfo(nh.Conf.NatsStream)
	if stream != nil {
		if _, err := js.AddStream(&nats.StreamConfig{
			Name: nh.Conf.NatsStream,
			Subjects: []string{
				fmt.Sprintf("%s.notification", nh.Conf.NatsStream),
				fmt.Sprintf("%s.trigger", nh.Conf.NatsStream),
			},
		}); err != nil {
			return errors.Wrap(err, "nats stream creation failed")
		}
	}
	nh.js = js

	bucketName := fmt.Sprintf("%s-template", nh.Conf.NatsStream)
	kv, err := nh.js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		return errors.Wrap(err, "template store creation failed, probably because nats server is unreachable")
	}
	nh.kv = kv
	return nil
}

// Stop stops the notification helper.
func (nh *NotificationHelper) Stop() {
	if err := nh.nc.Drain(); err != nil {
		nh.Log.Error().Err(err)
	}
}

func (nh *NotificationHelper) registerTemplates(ts map[string]interface{}) {
	if len(ts) == 0 {
		nh.Log.Info().Msg("no templates to register")
		return
	}

	tCount := 0
	for tn, tm := range ts {
		var tc template.RegistrationRequest
		if err := mapstructure.Decode(tm, &tc); err != nil {
			nh.Log.Error().Err(err).Msgf("template '%s' definition decoding failed", tn)
			continue
		}
		if err := template.CheckTemplateName(tc.Name); err != nil {
			nh.Log.Error().Err(err).Msgf("template name '%s' is incorrect", tc.Name)
			continue
		}
		if tc.Handler == "" {
			nh.Log.Error().Msgf("template definition '%s' is missing handler field", tn)
			continue
		}
		if tc.BodyTmplPath == "" {
			nh.Log.Error().Msgf("template definition '%s' is missing body_template_path field", tn)
			continue
		}

		nh.registerTemplate(&tc)
		tCount++
	}

	nh.Log.Info().Msgf("%d templates to register", tCount)
}

func (nh *NotificationHelper) registerTemplate(rr *template.RegistrationRequest) {
	if nh.kv == nil {
		nh.Log.Info().Msgf("template registration skipped, helper is misconfigured")
		return
	}

	tb, err := json.Marshal(rr)
	if err != nil {
		nh.Log.Error().Err(err).Msgf("template registration json marshalling failed")
	}

	go func() {
		_, err := nh.kv.Put(rr.Name, tb)
		if err != nil {
			nh.Log.Error().Err(err).Msgf("template registration publish failed")
			return
		}
		nh.Log.Debug().Msgf("%s template registration published", rr.Name)
	}()
}

// RegisterNotification registers a notification in the notification service.
func (nh *NotificationHelper) RegisterNotification(n *notification.Notification) {
	if nh.js == nil {
		nh.Log.Info().Msgf("notification registration skipped, helper is misconfigured")
		return
	}

	nb, err := json.Marshal(n)
	if err != nil {
		nh.Log.Error().Err(err).Msgf("notification registration json marshalling failed")
		return
	}

	notificationSubject := fmt.Sprintf("%s.notification-register", nh.Conf.NatsStream)

	go func() {
		_, err := nh.js.Publish(notificationSubject, nb)
		if err != nil {
			nh.Log.Error().Err(err).Msgf("notification registration publish failed")
			return
		}
		nh.Log.Debug().Msgf("%s notification registration published", n.Ref)
	}()
}

// UnregisterNotification unregisters a notification in the notification service.
func (nh *NotificationHelper) UnregisterNotification(ref string) {
	if nh.js == nil {
		nh.Log.Info().Msgf("notification unregistration skipped, notification helper is misconfigured")
		return
	}

	notificationSubject := fmt.Sprintf("%s.notification-unregister", nh.Conf.NatsStream)

	go func() {
		_, err := nh.js.Publish(notificationSubject, []byte(ref))
		if err != nil {
			nh.Log.Error().Err(err).Msgf("notification unregistration publish failed")
			return
		}
		nh.Log.Debug().Msgf("%s notification unregistration published", ref)
	}()
}

// TriggerNotification sends a notification trigger to the notifications service.
func (nh *NotificationHelper) TriggerNotification(tr *trigger.Trigger) {
	if nh.js == nil {
		nh.Log.Info().Msgf("notification trigger skipped, notification helper is misconfigured")
		return
	}

	trb, err := json.Marshal(tr)
	if err != nil {
		nh.Log.Error().Err(err).Msgf("notification trigger json marshalling failed")
		return
	}

	triggerSubject := fmt.Sprintf("%s.trigger", nh.Conf.NatsStream)

	go func() {
		_, err := nh.js.Publish(triggerSubject, trb)
		if err != nil {
			nh.Log.Error().Err(err).Msgf("notification trigger publish failed")
			return
		}
		nh.Log.Debug().Msgf("%s notification trigger published", tr.Ref)
	}()
}
