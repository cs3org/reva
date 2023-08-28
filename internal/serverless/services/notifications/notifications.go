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

package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/notification"
	"github.com/cs3org/reva/pkg/notification/handler"
	handlerRegistry "github.com/cs3org/reva/pkg/notification/handler/registry"
	notificationManagerRegistry "github.com/cs3org/reva/pkg/notification/manager/registry"
	"github.com/cs3org/reva/pkg/notification/template"
	templateRegistry "github.com/cs3org/reva/pkg/notification/template/registry"
	"github.com/cs3org/reva/pkg/notification/trigger"
	"github.com/cs3org/reva/pkg/notification/utils"
	pool "github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rserverless"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/accumulator"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type config struct {
	NatsAddress      string                            `mapstructure:"nats_address" docs:";The NATS server address."`
	NatsToken        string                            `mapstructure:"nats_token" docs:"The token to authenticate against the NATS server"`
	NatsPrefix       string                            `mapstructure:"nats_prefix" docs:"reva-notifications;The notifications NATS stream."`
	HandlerConf      map[string]interface{}            `mapstructure:"handlers" docs:";Settings for the different notification handlers."`
	GroupingInterval int                               `mapstructure:"grouping_interval" docs:"60;Time in seconds to group incoming notification triggers"`
	GroupingMaxSize  int                               `mapstructure:"grouping_max_size" docs:"100;Maximum number of notifications to group"`
	StorageDriver    string                            `mapstructure:"storage_driver" docs:"mysql;The driver used to store notifications"`
	StorageDrivers   map[string]map[string]interface{} `mapstructure:"storage_drivers"`
	GatewayAddress   string                            `mapstructure:"gatewaysvc"`
}

func defaultConfig() *config {
	return &config{
		NatsPrefix:       "reva-notifications",
		GroupingInterval: 60,
		GroupingMaxSize:  100,
		StorageDriver:    "sql",
	}
}

type svc struct {
	nc           *nats.Conn
	js           nats.JetStreamContext
	kv           nats.KeyValue
	conf         *config
	log          *zerolog.Logger
	handlers     map[string]handler.Handler
	templates    templateRegistry.Registry
	nm           notification.Manager
	accumulators map[string]*accumulator.Accumulator[trigger.Trigger]
}

func init() {
	rserverless.Register("notifications", New)
}

func getNotificationManager(c *config, l *zerolog.Logger) (notification.Manager, error) {
	if f, ok := notificationManagerRegistry.NewFuncs[c.StorageDriver]; ok {
		return f(c.StorageDrivers[c.StorageDriver])
	}
	return nil, errtypes.NotFound(fmt.Sprintf("storage driver %s not found", c.StorageDriver))
}

// New returns a new Notifications service.
func New(m map[string]interface{}, log *zerolog.Logger) (rserverless.Service, error) {
	conf := defaultConfig()

	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	nm, err := getNotificationManager(conf, log)
	if err != nil {
		return nil, err
	}
	log.Info().Msgf("notification storage %s initialized", conf.StorageDriver)

	s := &svc{
		conf: conf,
		log:  log,
		nm:   nm,
	}

	return s, nil
}

// Start starts the Notifications service.
func (s *svc) Start() {
	s.templates = *templateRegistry.New()
	s.handlers = handlerRegistry.InitHandlers(s.conf.HandlerConf, s.log)
	s.accumulators = make(map[string]*accumulator.Accumulator[trigger.Trigger])

	s.log.Debug().Msgf("connecting to nats server at %s", s.conf.NatsAddress)
	err := s.connect()
	if err != nil {
		s.log.Error().Err(err).Msg("connecting to nats failed")
	}
	s.log.Info().Msg("notifications service ready")
}

// Close performs cleanup.
func (s *svc) Close(ctx context.Context) error {
	return s.nc.Drain()
}

func (s *svc) connect() error {
	nc, err := utils.ConnectToNats(s.conf.NatsAddress, s.conf.NatsToken, *s.log)
	if err != nil {
		return err
	}
	s.nc = nc

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return errors.Wrap(err, "jetstream initialization failed")
	}

	s.js = js

	if err := s.initNatsKV("template", s.handleMsgTemplate); err != nil {
		return err
	}
	if err := s.initNatsStream("notification-register", s.handleMsgRegisterNotification); err != nil {
		return err
	}
	if err := s.initNatsStream("notification-unregister", s.handleMsgUnregisterNotification); err != nil {
		return err
	}
	return s.initNatsStream("trigger", s.handleMsgTrigger)
}

func (s *svc) initNatsKV(name string, handler func(msg []byte)) error {
	bucketName := fmt.Sprintf("%s-%s", s.conf.NatsPrefix, name)
	kv, err := s.js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		return errors.Wrap(err, "template store creation failed, probably because nats server is unreachable")
	}

	s.kv = kv

	w, _ := kv.WatchAll()

	go func() {
		for {
			msg := <-w.Updates()

			if msg != nil {
				handler(msg.Value())
			}
		}
	}()

	return nil
}

func (s *svc) initNatsStream(name string, handler func(msg *nats.Msg)) error {
	streamName := fmt.Sprintf("%s-%s", s.conf.NatsPrefix, name)
	consumerName := fmt.Sprintf("%s-consumer-%s", s.conf.NatsPrefix, name)
	subjectName := fmt.Sprintf("%s.%s", s.conf.NatsPrefix, name)
	deliverySubjectName := fmt.Sprintf("%s-delivery.%s", s.conf.NatsPrefix, name)

	// Creates a NATS stream with given name if it does not exist already
	if _, err := s.js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subjectName},
	}); err != nil {
		return errors.Wrapf(err, "nats %s stream creation failed", name)
	}

	// Adds a consumer with the given name to the JetStream context
	if _, err := s.js.AddConsumer(streamName, &nats.ConsumerConfig{
		Durable:        consumerName,
		DeliverSubject: deliverySubjectName,
	}); err != nil {
		return errors.Wrapf(err, "nats %s consumer creation failed", name)
	}

	// Subscribes the JetStream context to the consumer we just created
	_, err := s.js.Subscribe("", func(msg *nats.Msg) { handler(msg) }, nats.Bind(streamName, consumerName))
	if err != nil {
		return errors.Wrapf(err, "nats subscription to consumer %s failed", consumerName)
	}

	return nil
}

func (s *svc) handleMsgTemplate(msg []byte) {
	if len(msg) == 0 {
		return
	}

	name, err := s.templates.Put(msg, s.handlers)
	if err != nil {
		s.log.Error().Err(err).Msgf("template registration failed %v", err)

		// If a template file was not found, delete that template from the registry altogether,
		// this way we ensure templates that are deleted from the config are deleted from the
		// store too.
		wrappedErr := errors.Unwrap(errors.Unwrap(err))
		_, isFileNotFoundError := wrappedErr.(*template.FileNotFoundError)
		if isFileNotFoundError && name != "" {
			err := s.kv.Purge(name)
			if err != nil {
				s.log.Error().Err(err).Msgf("deletion of template %s from store failed", name)
			}
			s.log.Info().Msgf("template %s unregistered", name)
		}
	} else {
		s.log.Info().Msgf("template %s registered", name)
	}
}

func (s *svc) handleMsgRegisterNotification(msg *nats.Msg) {
	var data map[string]interface{}
	err := json.Unmarshal(msg.Data, &data)
	if err != nil {
		s.log.Error().Err(err).Msg("notification registration unmarshall failed")
		return
	}

	n := &notification.Notification{}
	if err := mapstructure.Decode(data, n); err != nil {
		s.log.Error().Err(err).Msg("notification registration decoding failed")
		return
	}

	templ, err := s.templates.Get(n.TemplateName)
	if err != nil {
		s.log.Error().Err(err).Msg("notification template get failed")
		return
	}

	n.Template = *templ
	err = s.nm.UpsertNotification(*n)
	if err != nil {
		s.log.Error().Err(err).Msgf("registering notification %s failed", n.Ref)
	} else {
		s.log.Info().Msgf("notification %s registered", n.Ref)
	}
}

func (s *svc) handleMsgUnregisterNotification(msg *nats.Msg) {
	ref := string(msg.Data)

	err := s.nm.DeleteNotification(ref)
	if err != nil {
		_, isNotFoundError := err.(*notification.NotFoundError)
		if isNotFoundError {
			s.log.Debug().Msgf("a notification with ref %s does not exist", ref)
		} else {
			s.log.Error().Err(err).Msgf("notification unregister failed")
		}
	} else {
		s.log.Debug().Msgf("notification %s unregistered", ref)
	}
}

func (s *svc) getAccumulatorForTrigger(tr trigger.Trigger) *accumulator.Accumulator[trigger.Trigger] {
	a, ok := s.accumulators[tr.Ref]

	if !ok || a == nil {
		timeout := time.Duration(s.conf.GroupingInterval) * time.Second
		maxSize := s.conf.GroupingMaxSize

		a = accumulator.New[trigger.Trigger](timeout, maxSize, s.log)
		_ = a.Start(s.notificationSendCallback)
		s.accumulators[tr.Ref] = a

		s.log.Debug().Msgf("created new accumulator for trigger %s", tr.Ref)
	}

	return a
}

func (s *svc) handleMsgTrigger(msg *nats.Msg) {
	var data map[string]interface{}
	err := json.Unmarshal(msg.Data, &data)
	if err != nil {
		s.log.Error().Err(err).Msg("notification trigger unmarshall failed")
		return
	}

	tr := &trigger.Trigger{}
	if err := mapstructure.Decode(data, tr); err != nil {
		s.log.Error().Err(err).Msg("trigger creation failed")
		return
	}

	s.log.Info().Msgf("notification trigger %s received", tr.Ref)

	notif := tr.Notification
	if notif == nil {
		notif, err = s.nm.GetNotification(tr.Ref)
		if err != nil {
			_, isNotFoundError := err.(*notification.NotFoundError)
			if isNotFoundError {
				s.log.Debug().Msgf("trigger %s does not have a notification attached", tr.Ref)
				return
			}
			s.log.Error().Err(err).Msgf("notification retrieval from store failed")
			return
		}
	}

	templ, err := s.templates.Get(notif.TemplateName)
	if err != nil {
		s.log.Error().Err(err).Msgf("template %s for trigger %s not found", notif.TemplateName, tr.Ref)
		return
	}

	notif.Template = *templ
	tr.Notification = notif
	a := s.getAccumulatorForTrigger(*tr)
	a.Input <- *tr
}

func (s *svc) notificationSendCallback(ts []trigger.Trigger) {
	const itemCount = 10
	var tr trigger.Trigger

	s.log.Info().Msgf("preparing notification for trigger %s", tr.Ref)

	if len(ts) == 1 {
		tr = ts[0]
	} else {
		moreCount := len(ts) - itemCount
		if moreCount < 0 {
			moreCount = 0
		}

		// create a new trigger
		tr = trigger.Trigger{
			Ref:    ts[0].Ref,
			Sender: ts[0].Sender,
			TemplateData: map[string]interface{}{
				"_count":     len(ts),
				"_items":     []map[string]interface{}{},
				"_moreCount": moreCount,
			},
		}

		// add template data of the first ten elements, ignore the rest
		l := itemCount
		templateData := []map[string]interface{}{}
		if l > len(ts) {
			l = len(ts)
		}
		for _, t := range ts[:l] {
			templateData = append(templateData, t.TemplateData)
		}
		tr.TemplateData["_items"] = templateData

		// initialize the new trigger
		notif, err := s.nm.GetNotification(tr.Ref)
		if err != nil {
			s.log.Error().Msgf("notification retrieval from store failed")
			return
		}

		templ, err := s.templates.Get(notif.TemplateName)
		if err != nil {
			s.log.Error().Err(err).Msgf("template %s for trigger %s not found", notif.TemplateName, tr.Ref)
			return
		}

		notif.Template = *templ
		tr.Notification = notif

		s.log.Info().Msgf("sending multi notification for %d triggers %s", tr.TemplateData["_count"], tr.Ref)
	}

	// destroy old accumulator
	s.accumulators[tr.Ref] = nil

	var filteredRecipients []string
	for _, recipientEmail := range tr.Notification.Recipients {
		ctx := context.Background()
		c, err := pool.GetGatewayServiceClient(pool.Endpoint(sharedconf.GetGatewaySVC(s.conf.GatewayAddress)))
		if err != nil {
			s.log.Error().Err(err).Msg("error getting grpc gateway client")
		}

		recipient, err := c.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim:                  "mail",
			Value:                  recipientEmail,
			SkipFetchingUserGroups: true,
		})
		if err != nil {
			s.log.Error().Err(err).Msgf("getting user by email failed for email %s", recipientEmail)
		}

		enabledNotifications, err := s.nm.GetNotificationPreference(recipient.User.Id.OpaqueId)
		if err != nil {
			fmt.Print("There was an error getting the notification preference")
			s.log.Error().Err(err).Msgf("getting notification preference for user failed, opaque_id %s", recipient.User.Id.OpaqueId)
		}

		if !enabledNotifications {
			filteredRecipients = append(filteredRecipients, recipientEmail)
		}
	}
	tr.Notification.Recipients = filteredRecipients

	if err := tr.Notification.CheckNotification(); err == nil {
		if err := tr.Send(); err != nil {
			s.log.Error().Err(err).Msgf("notification send failed")
		}
	}
}
