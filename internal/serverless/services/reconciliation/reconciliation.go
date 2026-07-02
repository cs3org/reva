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

// Package reconciliation hosts the share reconciliation jobs as a serverless
// service. The jobs span the share and the public link manager, which belong to
// two different grpc services, so they are wired here instead of in either of
// them.
package reconciliation

import (
	"context"
	"os"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	publicshareregistry "github.com/cs3org/reva/v3/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	shareregistry "github.com/cs3org/reva/v3/pkg/share/manager/registry"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func init() {
	rserverless.Register("reconciliation", New)
}

// The names a job is enabled and configured under.
const (
	jobOrphan = "orphan"
)

type config struct {
	// Jobs are the jobs to run, by name. Each one is configured in its own
	// section, so enabling a job means listing it here and giving it a
	// schedule.
	Jobs []string `mapstructure:"jobs"`
	// JWTSecret signs the token the jobs authenticate their own calls with.
	// Falls back to [shared].
	JWTSecret string `mapstructure:"jwt_secret"`
	// ServiceUserName is the account the jobs act as. It has to be known to the
	// storage: the EOS driver reads the ACLs of a node as the caller before it
	// hands them out.
	ServiceUserName string `mapstructure:"service_user_name" validate:"required"`
	// ServiceUserUID and ServiceUserGID are that account's ids. The EOS driver
	// refuses a caller without them, so neither may be zero.
	ServiceUserUID int64 `mapstructure:"service_user_uid" validate:"required"`
	ServiceUserGID int64 `mapstructure:"service_user_gid" validate:"required"`
	// ShareDriver and PublicShareDriver name the managers the jobs reconcile,
	// the same drivers the usershareprovider and the publicshareprovider run
	// on. Their configuration falls back to [shared], so it normally does not
	// have to be repeated here.
	ShareDriver        string                    `mapstructure:"share_driver"`
	ShareDrivers       map[string]map[string]any `mapstructure:"share_drivers"`
	PublicShareDriver  string                    `mapstructure:"publicshare_driver"`
	PublicShareDrivers map[string]map[string]any `mapstructure:"publicshare_drivers"`
	// Orphan configures the orphan job.
	Orphan reconciliation.Config `mapstructure:"orphan"`
}

func (c *config) ApplyDefaults() {
	c.JWTSecret = sharedconf.GetJWTSecret(c.JWTSecret)
	// sql is the only driver that has an orphan flag to set, so it is the
	// default here rather than json, the default of the grpc services.
	if c.ShareDriver == "" {
		c.ShareDriver = "sql"
	}
	if c.PublicShareDriver == "" {
		c.PublicShareDriver = "sql"
	}
	// a file per job, so a run of one is never interleaved with a run of
	// another and either can be pointed somewhere else on its own.
	if c.Orphan.LogFile == "" {
		c.Orphan.LogFile = "/var/log/revad/reconciliation-orphan.log"
	}
}

type svc struct {
	// logs are the job log files, one per job that writes to a file rather than
	// to a standard stream.
	logs []*os.File
}

// New builds the reconciliation service. It registers the jobs right away
// rather than in Start, because every serverless service is constructed before
// any is started and the jobs runner reads the registered jobs when it starts.
func New(ctx context.Context, m map[string]any) (_ rserverless.Service, err error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	if len(c.Jobs) == 0 {
		return nil, errors.Errorf("reconciliation: no jobs enabled, set jobs to %q", jobOrphan)
	}

	shares, err := getShareStore(ctx, &c)
	if err != nil {
		return nil, err
	}
	links, err := getPublicLinkStore(ctx, &c)
	if err != nil {
		return nil, err
	}
	tokens, err := jwt.New(map[string]any{"secret": c.JWTSecret})
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: building the token manager")
	}
	// the jobs read and write across the whole namespace, so the token carries
	// the owner scope, the same one an interactive session gets.
	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: building the token scope")
	}
	identity := &serviceUser{
		tokens: tokens,
		scope:  scopes,
		user: &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: c.ServiceUserName,
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username:  c.ServiceUserName,
			UidNumber: c.ServiceUserUID,
			GidNumber: c.ServiceUserGID,
		},
	}

	log := appctx.GetLogger(ctx)
	s := &svc{}
	// a job that never got registered still opened its log, so a failure part
	// way through the list has files to give back.
	defer func() {
		if err != nil {
			_ = s.closeLogs()
		}
	}()

	for _, name := range c.Jobs {
		var jc reconciliation.Config
		switch name {
		case jobOrphan:
			jc = c.Orphan
		default:
			return nil, errors.Errorf("reconciliation: unknown job %q, want %q", name, jobOrphan)
		}
		if jc.Schedule == "" {
			return nil, errors.Errorf("reconciliation: job %q has no schedule", name)
		}

		jobLog, logFile, err := reconciliation.OpenLog(jc.LogFile)
		if err != nil {
			return nil, err
		}
		if logFile != nil {
			s.logs = append(s.logs, logFile)
		}

		var periodic rjobs.Periodic
		switch name {
		case jobOrphan:
			// Gateway is left nil on purpose: the job resolves it through the
			// service registry per run, since a gateway need not be registered
			// yet at the time the jobs are wired.
			job := &reconciliation.OrphanJob{
				Shares:     shares,
				Links:      links,
				Auth:       identity.authenticate,
				Log:        jobLog,
				DryRun:     jc.DryRun,
				RunOnStart: jc.RunOnStart,
			}
			periodic = job.Periodic(jc.Schedule)
		}

		if err := rjobs.RegisterPeriodic(periodic); err != nil {
			return nil, errors.Wrapf(err, "reconciliation: registering %s", periodic.Name)
		}
		log.Info().
			Str("job", periodic.Name).
			Str("schedule", jc.Schedule).
			Str("log_file", jc.LogFile).
			Bool("dry_run", jc.DryRun).
			Bool("run_on_start", jc.RunOnStart).
			Msg("reconciliation: job registered")
	}

	return s, nil
}

// getShareStore builds the configured share manager and takes the part of it
// the jobs need. Marking an item orphaned is not part of the CS3 share API, so
// a driver that does not implement it is a configuration error.
func getShareStore(ctx context.Context, c *config) (reconciliation.ShareStore, error) {
	f, ok := shareregistry.NewFuncs[c.ShareDriver]
	if !ok {
		return nil, errors.Errorf("reconciliation: share driver not found: %s", c.ShareDriver)
	}
	sm, err := f(ctx, c.ShareDrivers[c.ShareDriver])
	if err != nil {
		return nil, errors.Wrapf(err, "reconciliation: opening share driver %s", c.ShareDriver)
	}
	store, ok := sm.(reconciliation.ShareStore)
	if !ok {
		return nil, errors.Errorf("reconciliation: share driver %s cannot be reconciled, it cannot mark a share orphaned", c.ShareDriver)
	}
	return store, nil
}

// getPublicLinkStore is getShareStore for the public link manager.
func getPublicLinkStore(ctx context.Context, c *config) (reconciliation.PublicLinkStore, error) {
	f, ok := publicshareregistry.NewFuncs[c.PublicShareDriver]
	if !ok {
		return nil, errors.Errorf("reconciliation: public share driver not found: %s", c.PublicShareDriver)
	}
	pm, err := f(ctx, c.PublicShareDrivers[c.PublicShareDriver])
	if err != nil {
		return nil, errors.Wrapf(err, "reconciliation: opening public share driver %s", c.PublicShareDriver)
	}
	store, ok := pm.(reconciliation.PublicLinkStore)
	if !ok {
		return nil, errors.Errorf("reconciliation: public share driver %s cannot be reconciled, it cannot mark a link orphaned", c.PublicShareDriver)
	}
	return store, nil
}

// serviceUser is the identity the job acts as. The jobs runner hands a run a
// bare context, and every gateway and storage provider call goes through the
// auth interceptor, which rejects a call without a token.
type serviceUser struct {
	tokens token.Manager
	user   *userpb.User
	scope  map[string]*authpb.Scope
}

// authenticate returns ctx carrying a token for the service user. It is minted
// per run rather than at startup because a token expires and this job is
// scheduled days apart.
func (s *serviceUser) authenticate(ctx context.Context) (context.Context, error) {
	tkn, err := s.tokens.MintToken(ctx, s.user, s.scope)
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: minting the service token")
	}
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn), nil
}

// Start is a no-op: the jobs service owns the runner that fires the jobs.
func (s *svc) Start() {}

// Close closes the job logs. The runner is stopped by the jobs service, so no
// run is in flight by the time this is called.
func (s *svc) Close(ctx context.Context) error {
	return s.closeLogs()
}

// closeLogs closes every log opened so far and reports the first failure.
func (s *svc) closeLogs() error {
	var err error
	for _, f := range s.logs {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	s.logs = nil
	return err
}
