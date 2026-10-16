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

package reconciliation

import (
	"context"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

const ExpiredJobName = "reconciliation.expired"

const (
	EventExpiredStart  = ExpiredJobName + ".start"
	EventExpiredRemove = ExpiredJobName + ".remove"
	EventExpiredFail   = ExpiredJobName + ".fail"
	EventExpiredEnd    = ExpiredJobName + ".end"
)

type ExpiredShareStore interface {
	ListExpiredShares(ctx context.Context) ([]*collaboration.Share, error)
	ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error)
	Unshare(ctx context.Context, ref *collaboration.ShareReference) error
}

// ExpiredGrantStore can remove an entry, unlike GrantStore. The job only
// removes the entry of a share it knows has expired.
type ExpiredGrantStore interface {
	AddGrant(ctx context.Context, in *provider.AddGrantRequest, opts ...grpc.CallOption) (*provider.AddGrantResponse, error)
	DenyGrant(ctx context.Context, in *provider.DenyGrantRequest, opts ...grpc.CallOption) (*provider.DenyGrantResponse, error)
	RemoveGrant(ctx context.Context, in *provider.RemoveGrantRequest, opts ...grpc.CallOption) (*provider.RemoveGrantResponse, error)
}

// ExpiredJob removes expired shares the way the gateway's RemoveShare does.
// The share manager hides an expired share, but its ACL entry stays on the
// storage, so the recipient keeps access until the job runs.
type ExpiredJob struct {
	ShareStore ExpiredShareStore
	Gateway    gateway.GatewayAPIClient
	Auth       func(ctx context.Context) (context.Context, error)
	Grants     func(ctx context.Context, storageID string) (ExpiredGrantStore, error)
	Log        *zerolog.Logger
	DryRun     bool
	RunOnStart bool
}

type ExpiredReport struct {
	RunID   string
	Expired int
	Removed []string
	Failed  int
	DryRun  bool
}

func (j *ExpiredJob) Run(ctx context.Context) (ExpiredReport, error) {
	base := j.Log
	if base == nil {
		base = appctx.GetLogger(ctx)
	}
	runID := uuid.New().String()
	l := base.With().Str("job", ExpiredJobName).Str("run", runID).Logger()
	log := &l

	if j.Auth != nil {
		var err error
		if ctx, err = j.Auth(ctx); err != nil {
			return ExpiredReport{}, err
		}
	}

	gw := j.Gateway
	if gw == nil {
		var err error
		if gw, err = service.Gateway(ctx); err != nil {
			return ExpiredReport{}, err
		}
	}

	expired, err := j.ShareStore.ListExpiredShares(ctx)
	if err != nil {
		return ExpiredReport{}, errors.Wrap(err, "reconciliation: listing expired shares")
	}

	log.Info().
		Str("event", EventExpiredStart).
		Bool("dry_run", j.DryRun).
		Int("candidates", len(expired)).
		Msg("reconciliation: run started")

	report := ExpiredReport{RunID: runID, Expired: len(expired), DryRun: j.DryRun}
	grantees := map[string]*provider.Grantee{}
	for _, s := range expired {
		traceID := trace.Generate()
		sharee, granteeType := sharehierarchy.ShareeInfo(s.GetGrantee())
		e := log.Info()
		if err := j.dropAclForExpiredShare(trace.Set(ctx, traceID), gw, grantees, s); err != nil {
			report.Failed++
			e = log.Error().Err(err).Str("event", EventExpiredFail)
		} else {
			report.Removed = append(report.Removed, s.GetId().GetOpaqueId())
			e = e.Str("event", EventExpiredRemove)
		}
		e.Str("share", s.GetId().GetOpaqueId()).
			Str("traceid", traceID).
			Str("storage_id", s.GetResourceId().GetStorageId()).
			Str("opaque_id", s.GetResourceId().GetOpaqueId()).
			Str("grantee", sharee).
			Str("grantee_type", granteeType).
			Str("level", sharehierarchy.PermLevelFromCS3(s.GetPermissions().GetPermissions()).String()).
			Bool("dry_run", j.DryRun).
			Msg("reconciliation: expired share processed")
	}

	log.Info().
		Str("event", EventExpiredEnd).
		Bool("dry_run", j.DryRun).
		Int("expired", report.Expired).
		Int("removed", len(report.Removed)).
		Int("failed", report.Failed).
		Msg("reconciliation: run finished")

	return report, nil
}

func (j *ExpiredJob) dropAclForExpiredShare(ctx context.Context, gw gateway.GatewayAPIClient, grantees map[string]*provider.Grantee, s *collaboration.Share) error {
	grantee, err := verifyGrantee(ctx, gw, grantees, s.GetGrantee())
	if err != nil {
		return err
	}

	space := s.GetResourceId().GetSpaceId()
	if space == "" {
		res, err := gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: s.GetResourceId()}})
		if err != nil {
			return errors.Wrap(err, "reconciliation: stat")
		}
		if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
			return errors.Errorf("reconciliation: stat: %s: %s", code, res.GetStatus().GetMessage())
		}
		if space = res.GetInfo().GetId().GetSpaceId(); space == "" {
			return errors.New("reconciliation: no space id for the shared resource")
		}
	}

	others, err := j.ShareStore.ListShares(ctx, []*collaboration.Filter{
		revashare.SpaceIDFilter(space),
		revashare.GranteeFilter(s.GetGrantee()),
	})
	if err != nil {
		return errors.Wrap(err, "reconciliation: listing the recipient's shares")
	}
	now := time.Now()
	sharee, granteeType := sharehierarchy.ShareeInfo(s.GetGrantee())
	related := []*collaboration.Share{s}
	for _, o := range others {
		if o.GetId().GetOpaqueId() == s.GetId().GetOpaqueId() || isExpired(o, now) {
			continue
		}
		// the grantee filter matches on the name only, so a group and a user
		// with the same name both come back.
		if n, t := sharehierarchy.ShareeInfo(o.GetGrantee()); n != sharee || t != granteeType {
			continue
		}
		related = append(related, o)
	}

	// the hierarchy check silently drops a share whose path does not resolve,
	// so every path is resolved up front and served from here.
	paths := make(map[string]string, len(related))
	for _, r := range related {
		res, err := gw.GetPath(ctx, &provider.GetPathRequest{ResourceId: r.GetResourceId()})
		if err != nil {
			return errors.Wrap(err, "reconciliation: get path")
		}
		if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
			return errors.Errorf("reconciliation: get path of share %s: %s: %s", r.GetId().GetOpaqueId(), code, res.GetStatus().GetMessage())
		}
		paths[spaces.ResourceIdToString(r.GetResourceId())] = res.GetPath()
	}
	checker := &sharehierarchy.Checker{GetPath: func(_ context.Context, id *provider.ResourceId) (string, error) {
		return paths[spaces.ResourceIdToString(id)], nil
	}}
	reapply := checker.GrantsToReapplyAfterRemove(ctx, s.GetId().GetOpaqueId(), s.GetResourceId(), related)

	if j.DryRun {
		return nil
	}

	store, err := j.Grants(ctx, s.GetResourceId().GetStorageId())
	if err != nil {
		return errors.Wrapf(err, "reconciliation: storage provider for %q", s.GetResourceId().GetStorageId())
	}
	res, err := store.RemoveGrant(ctx, &provider.RemoveGrantRequest{
		Ref:   &provider.Reference{ResourceId: s.GetResourceId()},
		Grant: &provider.Grant{Grantee: grantee, Permissions: s.GetPermissions().GetPermissions()},
	})
	if err != nil {
		return errors.Wrap(err, "reconciliation: remove grant")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return errors.Errorf("reconciliation: remove grant: %s: %s", code, res.GetStatus().GetMessage())
	}

	// the removal is recursive, so it also took the parent's entry on this
	// subtree and the entries of the shares below.
	restore := reapply.ChildGrants
	if reapply.ParentGrant != nil {
		restore = append([]*collaboration.Share{{ResourceId: s.GetResourceId(), Permissions: reapply.ParentGrant.GetPermissions()}}, restore...)
	}
	for _, r := range restore {
		ref := &provider.Reference{ResourceId: r.GetResourceId()}
		perms := r.GetPermissions().GetPermissions()
		var st *rpc.Status
		if grants.PermissionsEqual(perms, &provider.ResourcePermissions{}) {
			res, err := store.DenyGrant(ctx, &provider.DenyGrantRequest{Ref: ref, Grantee: grantee})
			if err != nil {
				return errors.Wrap(err, "reconciliation: deny grant")
			}
			st = res.GetStatus()
		} else {
			res, err := store.AddGrant(ctx, &provider.AddGrantRequest{Ref: ref, Grant: &provider.Grant{Grantee: grantee, Permissions: perms}})
			if err != nil {
				return errors.Wrap(err, "reconciliation: add grant")
			}
			st = res.GetStatus()
		}
		if code := st.GetCode(); code != rpc.Code_CODE_OK {
			return errors.Errorf("reconciliation: reapply grant on %s: %s: %s", spaces.ResourceIdToString(r.GetResourceId()), code, st.GetMessage())
		}
	}

	if err := j.ShareStore.Unshare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: s.GetId()}}); err != nil {
		return errors.Wrap(err, "reconciliation: removing the share")
	}
	return nil
}

func isExpired(s *collaboration.Share, now time.Time) bool {
	exp := s.GetExpiration().GetSeconds()
	return exp > 0 && !time.Unix(int64(exp), 0).After(now)
}

func (j *ExpiredJob) Periodic(schedule string) rjobs.Periodic {
	return rjobs.Periodic{
		Name:       ExpiredJobName,
		Schedule:   schedule,
		Scope:      rjobs.ScopeLeader,
		Overlap:    rjobs.Skip,
		RunOnStart: j.RunOnStart,
		Run: func(ctx context.Context) error {
			_, err := j.Run(ctx)
			return err
		},
	}
}
