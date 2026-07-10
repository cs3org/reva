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

// Package notifications hosts the notification worker. It consumes accepted
// notification envelopes from NATS, coordinates accumulation through SQL, and
// dispatches each envelope to the configured delivery handlers.
package notifications

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	revadcfg "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	notificationspkg "github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/backends"
	"github.com/cs3org/reva/v3/pkg/notifications/handlers"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	rserverless.Register("notifications", New)
}

type config struct {
	NATS backends.NATSConfig `mapstructure:"nats"`

	// Keep the flat NATS keys accepted by existing configs while the backend
	// config is being moved under [serverless.services.notifications.nats].
	NATSAddress string `mapstructure:"nats_address"`
	NATSToken   string `mapstructure:"nats_token"`

	Database revadcfg.Database `mapstructure:",squash"`

	Handlers map[string]map[string]any `mapstructure:"handlers"`

	WorkerID              string `mapstructure:"worker_id"`
	LeaseDurationSeconds  int    `mapstructure:"lease_duration_seconds"`
	ReaperIntervalSeconds int    `mapstructure:"reaper_interval_seconds"`
	ReaperLimit           int    `mapstructure:"reaper_limit"`
	MaxRenderedItems      int    `mapstructure:"max_rendered_items"`
}

func (c *config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)

	if c.NATS.Address == "" {
		c.NATS.Address = c.NATSAddress
	}
	if c.NATS.Token == "" {
		c.NATS.Token = c.NATSToken
	}
	if c.WorkerID == "" {
		hostname, _ := os.Hostname()
		c.WorkerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}
	if c.LeaseDurationSeconds == 0 {
		c.LeaseDurationSeconds = 300
	}
	if c.ReaperIntervalSeconds == 0 {
		c.ReaperIntervalSeconds = 900
	}
	if c.ReaperLimit == 0 {
		c.ReaperLimit = 100
	}
}

type svc struct {
	conf     *config
	ctx      context.Context
	log      *zerolog.Logger
	db       *gorm.DB
	listener *backends.NATSListener
	worker   *notificationspkg.Worker
}

// New returns a new notifications service.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

	db, err := openDB(c.Database)
	if err != nil {
		return nil, fmt.Errorf("notifications: opening accumulator database failed: %w", err)
	}

	store, err := notificationspkg.NewGORMStore(db)
	if err != nil {
		closeDB(db)
		return nil, err
	}

	dispatcher, err := newDispatcher(ctx, c.Handlers)
	if err != nil {
		closeDB(db)
		return nil, err
	}

	worker, err := notificationspkg.NewWorker(store, dispatcher, notificationspkg.WorkerConfig{
		OwnerID:          c.WorkerID,
		LeaseDuration:    time.Duration(c.LeaseDurationSeconds) * time.Second,
		MaxRenderedItems: c.MaxRenderedItems,
	})
	if err != nil {
		closeDB(db)
		return nil, err
	}

	reaper := notificationspkg.NewReaper(worker, notificationspkg.ReaperConfig{
		Interval: time.Duration(c.ReaperIntervalSeconds) * time.Second,
		Limit:    c.ReaperLimit,
	})
	if err := notificationspkg.RegisterReaperJob(reaper); err != nil {
		closeDB(db)
		return nil, err
	}

	return &svc{
		conf:   &c,
		ctx:    ctx,
		log:    appctx.GetLogger(ctx),
		db:     db,
		worker: worker,
	}, nil
}

func newDispatcher(ctx context.Context, conf map[string]map[string]any) (*handlers.Dispatcher, error) {
	dispatcher := handlers.NewDispatcher()

	for name, handlerConf := range conf {
		switch name {
		case handlers.EmailHandlerName:
			handler, err := handlers.NewEmailHandler(ctx, handlerConf)
			if err != nil {
				return nil, err
			}
			dispatcher.Register(handler)
		default:
			return nil, fmt.Errorf("notifications: unsupported handler %q", name)
		}
	}

	return dispatcher, nil
}

// Start starts consuming notification envelopes from NATS.
func (s *svc) Start() {
	if s.conf.NATS.Address == "" {
		s.log.Error().Msg("notifications: nats address is required")
		return
	}

	listener, err := backends.NewNATSListener(s.conf.NATS, *s.log)
	if err != nil {
		s.log.Error().Err(err).Msg("notifications: connecting to nats failed")
		return
	}

	if err := listener.Start(s.ctx, func(ctx context.Context, envelope model.Envelope) error {
		return s.worker.Handle(ctx, envelope)
	}); err != nil {
		s.log.Error().Err(err).Msg("notifications: subscribing to nats failed")
		_ = listener.Close()
		return
	}

	s.listener = listener
	s.log.Info().Msg("notifications service ready")
}

// Close stops the notification listener and closes the accumulator database.
func (s *svc) Close(ctx context.Context) error {
	var errs []error
	if s.listener != nil {
		errs = append(errs, s.listener.Close())
	}
	errs = append(errs, closeDB(s.db))
	return errors.Join(errs...)
}

func openDB(c revadcfg.Database) (*gorm.DB, error) {
	gormCfg := &gorm.Config{}
	switch c.Engine {
	case "sqlite":
		return gorm.Open(sqlite.Open(c.DBName), gormCfg)
	default:
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	}
}

func closeDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
