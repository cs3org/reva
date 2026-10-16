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

// Package reminders runs the reminder kinds configured under
// [serverless.services.reminders.kinds] as jobs, and keeps their ledger.
package reminders

import (
	"context"
	"fmt"
	"slices"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revadcfg "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/reminders"
	"github.com/cs3org/reva/v3/pkg/reminders/registry"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	rserverless.Register("reminders", New)
}

type config struct {
	Database  revadcfg.Database `mapstructure:",squash"`
	JWTSecret string            `mapstructure:"jwt_secret"`
	// The service user is the identity of the runs. It has to be known to
	// the storage, with its real ids, because a kind can read resources as
	// this user.
	ServiceUserName string `mapstructure:"service_user_name" validate:"required"`
	ServiceUserUID  int64  `mapstructure:"service_user_uid"  validate:"required"`
	ServiceUserGID  int64  `mapstructure:"service_user_gid"  validate:"required"`
	// MailDomain makes an address from a name when the directory has no mail
	// for it, as for an e-group.
	MailDomain    string `mapstructure:"mail_domain"`
	RetentionDays int    `mapstructure:"retention_days"`
	// Kinds are the configured kinds, by registered name.
	Kinds map[string]map[string]any `mapstructure:"kinds"`
}

func (c *config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
	c.JWTSecret = sharedconf.GetJWTSecret(c.JWTSecret)
	if c.MailDomain == "" {
		c.MailDomain = "cern.ch"
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = 365
	}
}

type svc struct {
	conf    *config
	db      *gorm.DB
	ledger  *reminders.Ledger
	names   []string
	runners map[string]*reminders.Runner
	configs map[string]reminders.Config
	set     *invoke.Set
}

// New builds the configured kinds and registers each one as the job
// reminders.<name>: on demand always, on its schedule when it has one.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	db, err := openDB(c.Database)
	if err != nil {
		return nil, errors.Wrap(err, "reminders: opening the database")
	}
	s := &svc{conf: &c, db: db, ledger: &reminders.Ledger{DB: db}, runners: map[string]*reminders.Runner{}, configs: map[string]reminders.Config{}}
	if err := s.init(ctx); err != nil {
		_ = s.Close(ctx)
		return nil, err
	}
	return s, nil
}

func (s *svc) init(ctx context.Context) error {
	if err := s.ledger.Migrate(); err != nil {
		return errors.Wrap(err, "reminders: migrating the ledger")
	}
	auth, err := s.authenticator()
	if err != nil {
		return err
	}
	for name, m := range s.conf.Kinds {
		newKind, ok := registry.NewFuncs[name]
		if !ok {
			return fmt.Errorf("reminders: kind %q is not registered", name)
		}
		var kc reminders.Config
		if err := cfg.Decode(m, &kc); err != nil {
			return errors.Wrapf(err, "reminders: kind %s", name)
		}
		kind, err := newKind(ctx, m)
		if err != nil {
			return errors.Wrapf(err, "reminders: kind %s", name)
		}
		r := &reminders.Runner{Name: name, Kind: kind, Ledger: s.ledger, DryRun: kc.DryRun, Auth: auth, Publish: s.publish, Now: time.Now}
		job := "reminders." + name
		if err := rjobs.RegisterOnDemand(job, func(context.Context, map[string]any) (rjobs.Job, error) { return onDemand{r}, nil }); err != nil {
			return err
		}
		if kc.Schedule != "" {
			err := rjobs.RegisterPeriodic(rjobs.Periodic{
				Name:       job + ".scheduled",
				Schedule:   kc.Schedule,
				Scope:      rjobs.ScopeLeader,
				RunOnStart: kc.RunOnStart,
				Run: func(ctx context.Context) error {
					_, err := r.Run(ctx)
					return err
				},
			})
			if err != nil {
				return err
			}
		}
		s.names = append(s.names, name)
		s.runners[name] = r
		s.configs[name] = kc
		appctx.GetLogger(ctx).Info().Str("kind", name).Str("schedule", kc.Schedule).Bool("dry_run", kc.DryRun).Msg("reminders: kind registered")
	}
	slices.Sort(s.names)

	retention := time.Duration(s.conf.RetentionDays) * 24 * time.Hour
	err = rjobs.RegisterPeriodic(rjobs.Periodic{
		Name:     "reminders.purge",
		Schedule: "@daily",
		Scope:    rjobs.ScopeLeader,
		Run: func(ctx context.Context) error {
			_, err := s.ledger.Purge(ctx, time.Now().Add(-retention))
			return err
		},
	})
	if err != nil {
		return err
	}
	s.set = s.invocations()
	return nil
}

type onDemand struct{ r *reminders.Runner }

func (j onDemand) Run(ctx context.Context, _ rjobs.Params) (rjobs.Params, error) {
	report, err := j.r.Run(ctx)
	return rjobs.Params{"scanned": report.Scanned, "due": report.Due, "recipients": report.Recipients, "published": report.Published, "closed": report.Closed, "failures": report.Failures, "dry_run": report.DryRun}, err
}

// authenticator returns the function that gives a run the service user and a
// token for it. The token is minted per run because runs can be a day apart.
func (s *svc) authenticator() (func(context.Context) (context.Context, error), error) {
	tokens, err := jwt.New(map[string]any{"secret": s.conf.JWTSecret})
	if err != nil {
		return nil, errors.Wrap(err, "reminders: building the token manager")
	}
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, err
	}
	user := &userpb.User{
		Id:        &userpb.UserId{OpaqueId: s.conf.ServiceUserName, Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username:  s.conf.ServiceUserName,
		UidNumber: s.conf.ServiceUserUID,
		GidNumber: s.conf.ServiceUserGID,
	}
	return func(ctx context.Context) (context.Context, error) {
		tkn, err := tokens.MintToken(ctx, user, owner)
		if err != nil {
			return nil, errors.Wrap(err, "reminders: minting the service token")
		}
		// scope.ContextWithMachineScope, used to publish, needs the user in
		// the context.
		ctx = appctx.ContextSetUser(ctx, user)
		return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn), nil
	}, nil
}

// publish sends a notification through the gateway, as a reva daemon. A user
// gets it at the mail that the directory has; an e-group, or a user without
// one, at <name>@<mail_domain>.
func (s *svc) publish(ctx context.Context, event string, to reminders.Recipient, data map[string]any) error {
	gw, err := service.Gateway(ctx)
	if err != nil {
		return err
	}
	user := &userpb.User{Username: to.Name, Mail: to.Name + "@" + s.conf.MailDomain}
	if !to.Group {
		res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: "username", Value: to.Name, SkipFetchingUserGroups: true})
		if err == nil && res.GetStatus().GetCode() == rpc.Code_CODE_OK && res.GetUser().GetMail() != "" {
			user = res.GetUser()
		}
	}
	ctx, err = scope.ContextWithMachineScope(ctx)
	if err != nil {
		return err
	}
	res, err := gw.PublishEvent(ctx, &gateway.PublishEventRequest{Event: notifications.EncodeEvent(event, []*userpb.User{user}, data)})
	if err != nil {
		return err
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return fmt.Errorf("the gateway refused the event: %s", res.GetStatus().GetMessage())
	}
	return nil
}

// Start does nothing: the jobs service runs the jobs.
func (s *svc) Start() {}

func (s *svc) Close(context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func openDB(c revadcfg.Database) (*gorm.DB, error) {
	if c.Engine == "sqlite" {
		return gorm.Open(sqlite.Open(c.DBName), &gorm.Config{})
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}
