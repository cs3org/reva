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
	"strconv"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// ShallowJobName is the stable identity of the shallow job.
const ShallowJobName = "reconciliation.shallow"

// The events the shallow job logs, see the package comment.
const (
	// EventShallowStart opens a run.
	EventShallowStart = ShallowJobName + ".start"
	// EventShallowGrant reports one grant written to a shared path, or, in
	// dry-run, one that would have been. It carries the permissions found and
	// the ones written, so the change can be undone.
	EventShallowGrant = ShallowJobName + ".grant"
	// EventShallowSkip reports one share left untouched, either because a
	// lookup failed or because an ancestor share shadows it.
	EventShallowSkip = ShallowJobName + ".skip"
	// EventShallowFail reports one grant that had to be written but could not
	// be.
	EventShallowFail = ShallowJobName + ".fail"
	// EventShallowEnd closes a run with its totals.
	EventShallowEnd = ShallowJobName + ".end"
)

// GrantStore is the subset of the CS3 storage provider API the shallow job
// needs. The grant calls are not part of the gateway API, so the job addresses
// the provider that hosts the resource directly, the way the gateway does
// internally; provider.ProviderAPIClient satisfies it.
type GrantStore interface {
	ListGrants(ctx context.Context, in *provider.ListGrantsRequest, opts ...grpc.CallOption) (*provider.ListGrantsResponse, error)
	AddGrant(ctx context.Context, in *provider.AddGrantRequest, opts ...grpc.CallOption) (*provider.AddGrantResponse, error)
	UpdateGrant(ctx context.Context, in *provider.UpdateGrantRequest, opts ...grpc.CallOption) (*provider.UpdateGrantResponse, error)
}

// ShallowJob reconciles the ACLs implied by the share database against the ones
// actually set on each shared path. It only visits the paths that carry a
// share, so its cost scales with the number of shares rather than with the size
// of the namespace, and drift anywhere else is left to the full-namespace
// sweep.
//
// It only ever adds a missing grant or corrects a wrong one. It never removes
// anything: an entry no share accounts for may still be a default ACL or an
// entry set outside CERNBox, and telling those apart needs the whole namespace.
// Which entries have to exist at all is decided by sharehierarchy, so nesting
// is handled by the same code that handles it when a share is created.
type ShallowJob struct {
	// Shares is the share store to reconcile.
	Shares ShareStore
	// Gateway resolves the path of a shared resource and the identity of its
	// recipient.
	Gateway gateway.GatewayAPIClient
	// Auth puts the identity the job acts as on the run context. The jobs
	// runner hands a run a bare context and both the gateway and the storage
	// providers reject a call without a token, so every run needs one. A nil
	// Auth leaves the context alone.
	Auth func(ctx context.Context) (context.Context, error)
	// Grants returns the grant API of the storage provider hosting storageID.
	// It is a function because the grant calls are not part of the gateway API,
	// so the provider hosting the resource has to be looked up first, which is
	// wiring this package stays out of.
	Grants func(ctx context.Context, storageID string) (GrantStore, error)
	// Log is the job's own log, see OpenLog. When nil the job falls back to the
	// logger in the run context.
	Log *zerolog.Logger
	// DryRun, when set, reports the grants it would write without writing any.
	DryRun bool
	// RunOnStart, when set, fires the job once as soon as the runner starts.
	RunOnStart bool
}

// ActionKind is what the job did to an ACL entry. There is no remove: the job
// only ever adds one that is missing or corrects one that is wrong.
type ActionKind int

const (
	// ActionAdd adds an entry that is missing.
	ActionAdd ActionKind = iota
	// ActionUpdate changes the permissions of an entry that is present.
	ActionUpdate
)

// String returns a human readable name for the action kind.
func (k ActionKind) String() string {
	switch k {
	case ActionAdd:
		return "add"
	case ActionUpdate:
		return "update"
	default:
		return "unknown"
	}
}

// WrittenGrant records one grant the job wrote, or, in dry-run, would have.
type WrittenGrant struct {
	// ShareID is the CS3 opaque id of the share that implies the grant.
	ShareID string
	// Path is the path the grant applies to.
	Path string
	// ResourceID is the node the grant applies to.
	ResourceID *provider.ResourceId
	// Grantee is the recipient: a username, a group name or an external id.
	Grantee string
	// GranteeType is what the recipient is, "user" or "group".
	GranteeType string
	// Action is ActionAdd when there was no entry at all and ActionUpdate when
	// there was one with the wrong permissions.
	Action ActionKind
	// Observed is the permission level found on the storage, empty when there
	// was no entry.
	Observed string
	// Expected is the permission level the share implies.
	Expected string
}

// ShallowReport summarises a run.
type ShallowReport struct {
	// RunID identifies the run. Every log line it wrote carries it under "run".
	RunID string
	// Checked is the number of shares whose implied entry was resolved.
	Checked int
	// Covered is the number of shares that needed no entry of their own because
	// an ancestor share already grants the same access.
	Covered int
	// Conflicting is the number of shares that grant less than a share on a
	// path above them. The hierarchy check refuses to create those, so they are
	// left alone rather than enforced.
	Conflicting int
	// Skipped is the number of shares left alone because a lookup failed.
	Skipped int
	// Failed is the number of grants that had to be written but could not be.
	Failed int
	// Written lists the grants written (or, in dry-run, that would be written).
	Written []WrittenGrant
	// DryRun reports whether the run was a simulation.
	DryRun bool
}

// Run reconciles every non-orphan share against the storage. A per-share lookup
// failure is logged and the share is skipped, never guessed at, so a flaky
// gateway can never cause a wrong ACL to be written. The run itself only fails
// if the shares cannot be listed at all.
//
// Every write is logged with the permissions found and the permissions written,
// so a run can be undone from its log alone.
func (j *ShallowJob) Run(ctx context.Context) (ShallowReport, error) {
	base := j.Log
	if base == nil {
		base = appctx.GetLogger(ctx)
	}
	runID := uuid.New().String()
	l := base.With().Str("job", ShallowJobName).Str("run", runID).Logger()
	log := &l

	if j.Auth != nil {
		var err error
		if ctx, err = j.Auth(ctx); err != nil {
			return ShallowReport{}, err
		}
	}

	shares, err := j.Shares.ListModelShares(nil, nil, true)
	if err != nil {
		return ShallowReport{}, errors.Wrap(err, "reconciliation: listing shares")
	}

	log.Info().
		Str("event", EventShallowStart).
		Bool("dry_run", j.DryRun).
		Int("candidates", len(shares)).
		Msg("reconciliation: run started")

	report := ShallowReport{RunID: runID, DryRun: j.DryRun}
	// the shares are grouped the way the hierarchy check expects them: all the
	// live shares of one recipient in one space. Spaces are disjoint, so a share
	// in another space never shadows one here.
	groups := map[string][]sharehierarchy.ResolvedShare{}
	var order []string
	paths := map[string]string{}
	for i := range shares {
		rs, err := j.resolve(ctx, &shares[i])
		if err != nil {
			report.Skipped++
			log.Error().Err(err).
				Str("event", EventShallowSkip).
				Str("share", strconv.FormatUint(uint64(shares[i].Id), 10)).
				Msg("reconciliation: share could not be resolved, left untouched")
			continue
		}
		report.Checked++
		paths[spaces.ResourceIdToString(rs.Share.GetResourceId())] = rs.Path
		sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())
		key := shares[i].SpaceID + "\x00" + granteeType + "\x00" + sharee
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], rs)
	}

	// the hierarchy check resolves the path of every share it is given. They are
	// all resolved already, so it is served from what the run has, and no share
	// is ever silently skipped for an unresolvable path.
	checker := &sharehierarchy.Checker{
		GetPath: func(_ context.Context, id *provider.ResourceId) (string, error) {
			p, ok := paths[spaces.ResourceIdToString(id)]
			if !ok {
				return "", errors.Errorf("reconciliation: no path resolved for %s", spaces.ResourceIdToString(id))
			}
			return p, nil
		},
	}

	// depth-first, so a share is always preceded by the shares above it and
	// required comes out with parents before children: the storage is never
	// left granting more below a path than on it.
	var required []sharehierarchy.ResolvedShare
	for _, key := range order {
		group := groups[key]
		sharehierarchy.SortDepthFirst(group)

		// above holds the entries this recipient gets on the path down to the
		// share being looked at, deepest last. Only entries that were written
		// are on it, since a share that gets none is not inherited from, and
		// each one escalates beyond the one below it, so the deepest is the
		// most that any share under it already has. Comparing against that one
		// therefore settles it, without walking every pair.
		var above []sharehierarchy.ResolvedShare
		for _, rs := range group {
			for len(above) > 0 && !sharehierarchy.IsStrictAncestor(above[len(above)-1].Path, rs.Path) {
				above = above[:len(above)-1]
			}
			var nearest []*collaboration.Share
			if len(above) > 0 {
				nearest = []*collaboration.Share{above[len(above)-1].Share}
			}

			// the same check that runs when a share is created: an entry is only
			// needed where it escalates beyond what is above it. Anything else
			// is already inherited, or would override an ancestor with something
			// weaker and take away access that ancestor grants.
			_, err := checker.CheckGrantConsistency(ctx, rs.Path, rs.Share.GetPermissions().GetPermissions(), nearest)
			if err != nil {
				j.reportShadowed(log, &report, rs, err)
				continue
			}
			required = append(required, rs)
			above = append(above, rs)
		}
	}

	for _, rs := range required {
		id := rs.Share.GetResourceId()
		sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())
		expected := sharehierarchy.PermLevelFromCS3(rs.Share.GetPermissions().GetPermissions())

		level, present, err := j.observed(ctx, rs)
		if err != nil {
			report.Skipped++
			log.Error().Err(err).
				Str("event", EventShallowSkip).
				Str("share", rs.Share.GetId().GetOpaqueId()).
				Str("path", rs.Path).
				Msg("reconciliation: grants could not be read, path left untouched")
			continue
		}

		var observed string
		var action ActionKind
		switch {
		case !present:
			action = ActionAdd
		case level != expected:
			observed, action = level.String(), ActionUpdate
		default:
			log.Debug().
				Str("share", rs.Share.GetId().GetOpaqueId()).
				Str("path", rs.Path).
				Str("grantee", sharee).
				Str("grantee_type", granteeType).
				Msg("reconciliation: grant is correct")
			continue
		}

		if !j.DryRun {
			if err := j.write(ctx, rs, action); err != nil {
				report.Failed++
				log.Error().Err(err).
					Str("event", EventShallowFail).
					Str("share", rs.Share.GetId().GetOpaqueId()).
					Str("path", rs.Path).
					Str("grantee", sharee).
					Str("grantee_type", granteeType).
					Msg("reconciliation: writing the grant failed")
				continue
			}
		}

		report.Written = append(report.Written, WrittenGrant{
			ShareID:     rs.Share.GetId().GetOpaqueId(),
			Path:        rs.Path,
			ResourceID:  id,
			Grantee:     sharee,
			GranteeType: granteeType,
			Action:      action,
			Observed:    observed,
			Expected:    expected.String(),
		})
		log.Info().
			Str("event", EventShallowGrant).
			Str("share", rs.Share.GetId().GetOpaqueId()).
			Str("action", action.String()).
			Str("path", rs.Path).
			Str("storage_id", id.GetStorageId()).
			Str("opaque_id", id.GetOpaqueId()).
			Str("grantee", sharee).
			Str("grantee_type", granteeType).
			Str("observed", observed).
			Str("expected", expected.String()).
			Bool("dry_run", j.DryRun).
			Msg("reconciliation: grant written")
	}

	log.Info().
		Str("event", EventShallowEnd).
		Bool("dry_run", j.DryRun).
		Int("checked", report.Checked).
		Int("covered", report.Covered).
		Int("conflicting", report.Conflicting).
		Int("written", len(report.Written)).
		Int("skipped", report.Skipped).
		Int("failed", report.Failed).
		Msg("reconciliation: run finished")

	return report, nil
}

// reportShadowed counts and logs a share that gets no entry of its own because
// a share above it shadows it. Shadowed means the share above grants the same
// recipient the same permissions, so the entry is inherited; anything else
// means the database holds a share the hierarchy check would have rejected.
func (j *ShallowJob) reportShadowed(log *zerolog.Logger, report *ShallowReport, rs sharehierarchy.ResolvedShare, err error) {
	level := sharehierarchy.PermLevelFromCS3(rs.Share.GetPermissions().GetPermissions())
	sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())

	var conflict *sharehierarchy.HierarchyConflictError
	if !errors.As(err, &conflict) || len(conflict.ConflictingShares) == 0 {
		report.Skipped++
		log.Error().Err(err).
			Str("event", EventShallowSkip).
			Str("share", rs.Share.GetId().GetOpaqueId()).
			Str("path", rs.Path).
			Msg("reconciliation: hierarchy check failed, share left untouched")
		return
	}

	ancestor := conflict.ConflictingShares[0]
	if ancestor.PermissionType == level.RoleID() {
		report.Covered++
		log.Debug().
			Str("share", rs.Share.GetId().GetOpaqueId()).
			Str("path", rs.Path).
			Str("grantee", sharee).
			Str("grantee_type", granteeType).
			Str("ancestor", ancestor.ID).
			Msg("reconciliation: entry is inherited from an ancestor share")
		return
	}

	report.Conflicting++
	log.Warn().
		Str("event", EventShallowSkip).
		Str("reason", "shadowed-by-ancestor").
		Str("share", rs.Share.GetId().GetOpaqueId()).
		Str("path", rs.Path).
		Str("grantee", sharee).
		Str("grantee_type", granteeType).
		Str("level", level.String()).
		Str("ancestor", ancestor.ID).
		Str("ancestor_path", ancestor.Path).
		Str("ancestor_role", ancestor.PermissionType).
		Msg("reconciliation: " + conflict.Message)
}

// resolve turns a share into the ACL entry it implies, paired with the current
// path of its resource. It fails when the path or the recipient cannot be
// resolved, which leaves the share alone rather than writing an entry built on
// a guess.
func (j *ShallowJob) resolve(ctx context.Context, s *model.Share) (sharehierarchy.ResolvedShare, error) {
	res, err := j.Gateway.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: &provider.ResourceId{StorageId: s.Instance, OpaqueId: s.Inode},
	})
	if err != nil {
		return sharehierarchy.ResolvedShare{}, errors.Wrap(err, "reconciliation: get path")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return sharehierarchy.ResolvedShare{}, errors.Errorf("reconciliation: get path: %s: %s", code, res.GetStatus().GetMessage())
	}

	grantee, err := j.grantee(ctx, s)
	if err != nil {
		return sharehierarchy.ResolvedShare{}, err
	}

	// AsCS3Share does the conversion from the stored OCS permissions, so the job
	// reconciles against the same permissions the share API hands out.
	return sharehierarchy.ResolvedShare{Share: s.AsCS3Share(grantee), Path: res.GetPath()}, nil
}

// grantee builds the CS3 grantee of a share. A user grantee is resolved through
// the gateway rather than built from the stored name, because its user type is
// what tells the storage driver where the entry goes: a lightweight account is
// not written to the native ACLs. Unlike the share manager, which falls back to
// a primary account when the lookup fails, a failure here is an error: guessing
// the type would write an external account into the native ACLs.
func (j *ShallowJob) grantee(ctx context.Context, s *model.Share) (*provider.Grantee, error) {
	if s.SharedWithIsGroup {
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: s.ShareWith}},
		}, nil
	}

	res, err := j.Gateway.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username", Value: s.ShareWith, SkipFetchingUserGroups: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: get user")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return nil, errors.Errorf("reconciliation: get user %q: %s: %s", s.ShareWith, code, res.GetStatus().GetMessage())
	}
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: res.GetUser().GetId()},
	}, nil
}

// observed reads the access the storage currently grants the entry's recipient
// on its path. The second result is false when there is no entry for that
// recipient at all, which is not the same as one granting nothing: an entry
// with no permissions is an active denial.
func (j *ShallowJob) observed(ctx context.Context, rs sharehierarchy.ResolvedShare) (sharehierarchy.PermLevel, bool, error) {
	id := rs.Share.GetResourceId()
	store, err := j.Grants(ctx, id.GetStorageId())
	if err != nil {
		return 0, false, errors.Wrapf(err, "reconciliation: storage provider for %q", id.GetStorageId())
	}
	res, err := store.ListGrants(ctx, &provider.ListGrantsRequest{
		Ref: &provider.Reference{ResourceId: id},
	})
	if err != nil {
		return 0, false, errors.Wrap(err, "reconciliation: list grants")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return 0, false, errors.Errorf("reconciliation: list grants: %s: %s", code, res.GetStatus().GetMessage())
	}

	wantSharee, wantType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())
	for _, g := range res.GetGrants() {
		if sharee, granteeType := sharehierarchy.ShareeInfo(g.GetGrantee()); sharee != wantSharee || granteeType != wantType {
			continue
		}
		// the storage keeps a coarser permission set than CS3 does, so the two
		// are compared at the granularity the hierarchy is defined on.
		return sharehierarchy.PermLevelFromCS3(g.GetPermissions()), true, nil
	}
	return 0, false, nil
}

// write applies the entry to the storage.
func (j *ShallowJob) write(ctx context.Context, rs sharehierarchy.ResolvedShare, action ActionKind) error {
	id := rs.Share.GetResourceId()
	store, err := j.Grants(ctx, id.GetStorageId())
	if err != nil {
		return errors.Wrapf(err, "reconciliation: storage provider for %q", id.GetStorageId())
	}
	ref := &provider.Reference{ResourceId: id}
	grant := &provider.Grant{
		Grantee:     rs.Share.GetGrantee(),
		Permissions: rs.Share.GetPermissions().GetPermissions(),
	}

	var st *rpc.Status
	switch action {
	case ActionAdd:
		res, err := store.AddGrant(ctx, &provider.AddGrantRequest{Ref: ref, Grant: grant})
		if err != nil {
			return errors.Wrap(err, "reconciliation: add grant")
		}
		st = res.GetStatus()
	case ActionUpdate:
		res, err := store.UpdateGrant(ctx, &provider.UpdateGrantRequest{Ref: ref, Grant: grant})
		if err != nil {
			return errors.Wrap(err, "reconciliation: update grant")
		}
		st = res.GetStatus()
	default:
		return errors.Errorf("reconciliation: unsupported action %q", action)
	}
	if code := st.GetCode(); code != rpc.Code_CODE_OK {
		return errors.Errorf("reconciliation: %s grant: %s: %s", action, code, st.GetMessage())
	}
	return nil
}

// Periodic wraps the job as an rjobs.Periodic. It runs on the leader because it
// mutates the storage, and skips a fire if the previous run is still going.
func (j *ShallowJob) Periodic(schedule string) rjobs.Periodic {
	return rjobs.Periodic{
		Name:       ShallowJobName,
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
