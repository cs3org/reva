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
	"sort"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// ShallowJobName is the stable identity of the shallow job, and the name a run
// is asked for by.
const ShallowJobName = "reconciliation.shallow"

// ShallowScheduleName is the name the scheduled full run is registered under.
// The jobs registry gives a name either a schedule or a trigger, not both.
const ShallowScheduleName = ShallowJobName + ".scheduled"

// The events the shallow job logs, see the package comment.
const (
	// EventShallowStart opens a run.
	EventShallowStart = ShallowJobName + ".start"
	// EventShallowGrant reports one grant written to a shared path, or, in
	// dry-run, one that would have been. It carries the permissions found and
	// the ones written, so the change can be undone.
	EventShallowGrant = ShallowJobName + ".grant"
	// EventShallowRemove reports one redundant share removed from the database,
	// or, in dry-run, one that would have been. It carries the share and the
	// ancestor that made it redundant, so the removal can be undone.
	EventShallowRemove = ShallowJobName + ".remove"
	// EventShallowSkip reports one share left untouched because a lookup
	// failed.
	EventShallowSkip = ShallowJobName + ".skip"
	// EventShallowSkipSpace reports one space left untouched because a share was
	// created in it while the run was checking, which makes what the run decided
	// stale. It carries the share that appeared.
	EventShallowSkipSpace = ShallowJobName + ".skipspace"
	// EventShallowFail reports one change, a grant or a removal, that had to be
	// made but could not be.
	EventShallowFail = ShallowJobName + ".fail"
	// EventShallowEnd closes a run with its totals.
	EventShallowEnd = ShallowJobName + ".end"
)

// GrantStore is the subset of the CS3 storage provider API the shallow job
// needs. The grant calls are not part of the gateway API, so the job addresses
// the provider that hosts the resource directly, the way the gateway does
// internally; provider.ProviderAPIClient satisfies it.
//
// RemoveGrant is deliberately not on it: the job has no way to remove an ACL
// entry, whatever it finds.
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
// an entry: an entry no share accounts for may still be a default ACL or an
// entry set outside CERNBox, and telling those apart needs the whole namespace.
// Which entries have to exist at all is decided by sharehierarchy, so nesting
// is handled by the same code that handles it when a share is created.
//
// A share the check finds no entry for because a share above it already grants
// its recipient at least as much is removed from the database, which is what
// creating that share above would have done to it. What goes is the share, and
// only the share: the ACLs on its path are left exactly as they are found. The
// row is soft deleted, so the removal can be undone.
//
// It is triggered on demand, over every space or a single one. A single space
// is a valid unit of work because spaces are disjoint: no share outside a space
// can shadow one inside it. A schedule is optional on top of that.
//
// A space is also the unit the changes are applied in. A run decides everything
// before it writes anything, and a user can share in the meantime, so the shares
// of a space are listed again just before its changes go out. A space that
// gained one since is left for the next run, whole: the new share is not in what
// the run compared, and it may well be what makes a share the run wants to
// remove no longer redundant.
type ShallowJob struct {
	ShareStore ShareStore
	// Gateway resolves the path of a shared resource and the identity of its
	// recipient. When nil the job resolves the gateway through the service
	// registry at the start of every run, which is what the serverless service
	// relies on: the registry may not hold a gateway yet when the job is
	// registered at startup.
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
	ShareID string
	// Path is the path the grant applies to.
	Path        string
	ResourceID  *provider.ResourceId
	Grantee     string
	GranteeType string
	Action      ActionKind
	// Observed is the permission level found on the storage, empty when there
	// was no entry.
	Observed string
	// Expected is the permission level the share implies.
	Expected string
}

// RedundantReason says why a share needed no entry of its own, and so why it
// was removed.
type RedundantReason string

const (
	// ReasonInherited means an ancestor share grants the same recipient exactly
	// the same access, so the share adds nothing to what is inherited.
	ReasonInherited RedundantReason = "inherited"
	// ReasonShadowedByAncestor means an ancestor share grants the same
	// recipient more, so the share cannot take effect: its entry would take
	// away access the ancestor grants.
	ReasonShadowedByAncestor RedundantReason = "shadowed-by-ancestor"
)

// RemovedShare records one redundant share the job removed, or, in dry-run,
// would have.
type RemovedShare struct {
	ShareID     string
	Path        string
	ResourceID  *provider.ResourceId
	Grantee     string
	GranteeType string
	// Level is the permission level the removed share granted.
	Level  string
	Reason RedundantReason
	// AncestorID is the ID of the share above it that made it redundant.
	AncestorID string
	// AncestorPath is the path of the share above it that made it redundant.
	AncestorPath string
	// AncestorRole is the role of the share above it that made it redundant.
	AncestorRole string
}

// ShallowReport summarises a run.
type ShallowReport struct {
	// RunID identifies the run. Every log line it wrote carries it under "run".
	RunID string
	// SpaceID is the space the run was limited to, empty for a full run.
	SpaceID string
	// Checked is the number of shares whose implied entry was resolved.
	Checked int
	// Covered is the number of shares that needed no entry of their own because
	// an ancestor share already grants the same access.
	Covered int
	// Conflicting is the number of shares that grant less than a share on a
	// path above them. The hierarchy check refuses to create those, so their
	// entry is never enforced.
	Conflicting int
	// Skipped is the number of shares left alone because a lookup failed.
	Skipped int
	// Failed is the number of changes, a grant or a removal, that had to be
	// made but could not be.
	Failed int
	// Written lists the grants written (or, in dry-run, that would be written).
	Written []WrittenGrant
	// Removed lists the redundant shares removed (or, in dry-run, that would be
	// removed). Every share counted covered or conflicting is on it, unless its
	// removal failed or its space was skipped.
	Removed []RemovedShare
	// SkippedSpaces lists the spaces nothing was applied to because a share
	// appeared in them while the run was checking. Their shares are counted
	// checked, covered and conflicting all the same, since the check did run on
	// them, but nothing they called for was written or removed.
	SkippedSpaces []string
	// DryRun reports whether the run was a simulation.
	DryRun bool
}

// Run reconciles every non-orphan share against the storage. A per-share lookup
// failure is logged and the share is skipped, never guessed at, so a flaky
// gateway can never cause a wrong ACL to be written. The run itself only fails
// if the shares cannot be listed at all.
//
// A space whose shares changed while the run was checking is skipped whole, and
// left to the next run.
//
// Every write is logged with the permissions found and the permissions written,
// so a run can be undone from its log alone.
func (j *ShallowJob) Run(ctx context.Context) (ShallowReport, error) {
	return j.run(ctx, "")
}

// RunSpace is Run over the shares of a single space.
func (j *ShallowJob) RunSpace(ctx context.Context, spaceID string) (ShallowReport, error) {
	if spaceID == "" {
		return ShallowReport{}, errors.New("reconciliation: no space given")
	}
	return j.run(ctx, spaceID)
}

// run does the work of both, over every space when spaceID is empty.
func (j *ShallowJob) run(ctx context.Context, spaceID string) (ShallowReport, error) {
	base := j.Log
	if base == nil {
		base = appctx.GetLogger(ctx)
	}
	runID := uuid.New().String()
	fields := base.With().Str("job", ShallowJobName).Str("run", runID)
	if spaceID != "" {
		fields = fields.Str("space", spaceID)
	}
	l := fields.Logger()
	log := &l

	if j.Auth != nil {
		var err error
		if ctx, err = j.Auth(ctx); err != nil {
			return ShallowReport{}, err
		}
	}

	// after Auth, so the resolved client is used with a context carrying the
	// job's token.
	gw, err := j.gateway(ctx)
	if err != nil {
		return ShallowReport{}, err
	}

	var filters []*collaboration.Filter
	if spaceID != "" {
		filters = append(filters, revashare.SpaceIDFilter(spaceID))
	}
	shares, err := j.ShareStore.ListShares(ctx, filters)
	if err != nil {
		return ShallowReport{}, errors.Wrap(err, "reconciliation: listing shares")
	}

	log.Info().
		Str("event", EventShallowStart).
		Bool("dry_run", j.DryRun).
		Int("candidates", len(shares)).
		Msg("reconciliation: run started")

	report := ShallowReport{RunID: runID, SpaceID: spaceID, DryRun: j.DryRun}
	// the shares are grouped the way the hierarchy check expects them: all the
	// live shares of one recipient in one space. The key is the space id, the
	// grantee type and the sharee, joined by a NUL byte, which no name contains.
	// Spaces are disjoint, so a share in another space never shadows one here.
	groups := map[string][]sharehierarchy.ResolvedShare{}
	paths := make(map[string]string, len(shares))
	// Each share gets its own trace id, propagated into every gateway and
	// storage provider call made for it by the pool's client interceptor. It is
	// logged on each per-share line, and the same share keeps it across both
	// passes, so a skip here can be joined against the revad logs that explain
	// it: grep the same traceid there.
	traces := map[string]string{}
	// one gateway lookup per recipient instead of one per share: a recipient
	// holds many shares, so the same names come back over and over.
	grantees := map[string]*provider.Grantee{}
	// the shares each space held when the check ran, by space, so a share that
	// appears under the run can be told from one the run has seen. A share that
	// does not resolve is on it too: it exists, it is only the run that cannot
	// judge it.
	seen := map[string]map[string]struct{}{}
	for _, s := range shares {
		id := s.GetId().GetOpaqueId()
		space := s.GetResourceId().GetSpaceId()
		if seen[space] == nil {
			seen[space] = map[string]struct{}{}
		}
		seen[space][id] = struct{}{}

		traceID := trace.Generate()
		traces[id] = traceID

		rs, err := j.resolve(trace.Set(ctx, traceID), gw, grantees, s)
		if err != nil {
			report.Skipped++
			log.Error().Err(err).
				Str("event", EventShallowSkip).
				Str("share", id).
				Str("traceid", traceID).
				Msg("reconciliation: share could not be resolved, left untouched")
			continue
		}
		report.Checked++
		paths[spaces.ResourceIdToString(rs.Share.GetResourceId())] = rs.Path
		sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())
		key := space + "\x00" + granteeType + "\x00" + sharee
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
	required := make([]sharehierarchy.ResolvedShare, 0, len(shares))
	var redundant []RemovedShare
	for _, group := range groups {
		sharehierarchy.SortDepthFirst(group)

		// above holds the entries this recipient gets on the path down to the
		// share being looked at, deepest last. Only entries that were written
		// are on it, since a share that gets none is not inherited from, and
		// each one escalates beyond the one below it, so the deepest is the
		// most that any share under it already has. Comparing against that one
		// therefore settles it, without walking every pair.
		var above []sharehierarchy.ResolvedShare
		for _, rs := range group {
			// drop the shares this one is not under: their subtree is done.
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
				if r, ok := j.shadowed(log, &report, rs, traces[rs.Share.GetId().GetOpaqueId()], err); ok {
					redundant = append(redundant, r)
				}
				continue
			}
			required = append(required, rs)
			above = append(above, rs)
		}
	}

	// what to write is decided against the storage before anything goes out, so
	// that the changes of a space can be checked against the database as a whole
	// and then applied in one go.
	planned := j.plan(ctx, log, &report, required, traces)

	// the changes are grouped per space, the unit a run applies or holds back.
	grants := map[string][]plannedGrant{}
	for _, p := range planned {
		space := p.share.Share.GetResourceId().GetSpaceId()
		grants[space] = append(grants[space], p)
	}
	removals := map[string][]RemovedShare{}
	for _, r := range redundant {
		space := r.ResourceID.GetSpaceId()
		removals[space] = append(removals[space], r)
	}
	// only the spaces something is held back for are visited, so a space the run
	// changes nothing in is never listed again: there is nothing a share created
	// in it can spoil. The order is fixed, so two runs over the same state read
	// the same way.
	pending := make([]string, 0, len(grants))
	for space := range grants {
		pending = append(pending, space)
	}
	for space := range removals {
		if _, ok := grants[space]; !ok {
			pending = append(pending, space)
		}
	}
	sort.Strings(pending)

	for _, space := range pending {
		fresh, err := j.appeared(ctx, space, seen[space])
		if err != nil {
			report.SkippedSpaces = append(report.SkippedSpaces, space)
			log.Error().Err(err).
				Str("event", EventShallowSkipSpace).
				Str("skipped_space", space).
				Int("grants", len(grants[space])).
				Int("removals", len(removals[space])).
				Bool("dry_run", j.DryRun).
				Msg("reconciliation: shares could not be listed again, space left untouched")
			continue
		}
		if fresh != "" {
			report.SkippedSpaces = append(report.SkippedSpaces, space)
			log.Warn().
				Str("event", EventShallowSkipSpace).
				Str("skipped_space", space).
				Str("new_share", fresh).
				Int("grants", len(grants[space])).
				Int("removals", len(removals[space])).
				Bool("dry_run", j.DryRun).
				Msg("reconciliation: a share was created while the run was checking, space left untouched")
			continue
		}

		j.applyGrants(ctx, log, &report, grants[space], traces)
		// the removals go last, so the entry a share is removed in favour of is
		// on the storage before its row is gone, the order the share API removes
		// a redundant share in. The two are always in the same space: a share is
		// only ever made redundant by one above it, and no share above it lives
		// anywhere else.
		j.applyRemovals(ctx, log, &report, removals[space], traces)
	}

	log.Info().
		Str("event", EventShallowEnd).
		Bool("dry_run", j.DryRun).
		Int("checked", report.Checked).
		Int("covered", report.Covered).
		Int("conflicting", report.Conflicting).
		Int("written", len(report.Written)).
		Int("removed", len(report.Removed)).
		Int("skipped", report.Skipped).
		Int("skipped_spaces", len(report.SkippedSpaces)).
		Int("failed", report.Failed).
		Msg("reconciliation: run finished")

	return report, nil
}

// plannedGrant is one grant a run decided on, held until the space it belongs
// to has been checked for shares created under the run.
type plannedGrant struct {
	share sharehierarchy.ResolvedShare
	entry WrittenGrant
}

// plan works out the grant each share needs from what the storage has now.
// Nothing is written here. A share whose grants cannot be read is skipped, the
// same as one that does not resolve.
func (j *ShallowJob) plan(ctx context.Context, log *zerolog.Logger, report *ShallowReport, required []sharehierarchy.ResolvedShare, traces map[string]string) []plannedGrant {
	planned := make([]plannedGrant, 0, len(required))
	for _, rs := range required {
		id := rs.Share.GetResourceId()
		sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())
		expected := sharehierarchy.PermLevelFromCS3(rs.Share.GetPermissions().GetPermissions())
		traceID := traces[rs.Share.GetId().GetOpaqueId()]

		level, present, err := j.observed(trace.Set(ctx, traceID), rs)
		if err != nil {
			report.Skipped++
			log.Error().Err(err).
				Str("event", EventShallowSkip).
				Str("share", rs.Share.GetId().GetOpaqueId()).
				Str("path", rs.Path).
				Str("traceid", traceID).
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
			log.Trace().
				Str("share", rs.Share.GetId().GetOpaqueId()).
				Str("path", rs.Path).
				Str("grantee", sharee).
				Str("grantee_type", granteeType).
				Str("traceid", traceID).
				Msg("reconciliation: grant is correct")
			continue
		}

		planned = append(planned, plannedGrant{share: rs, entry: WrittenGrant{
			ShareID:     rs.Share.GetId().GetOpaqueId(),
			Path:        rs.Path,
			ResourceID:  id,
			Grantee:     sharee,
			GranteeType: granteeType,
			Action:      action,
			Observed:    observed,
			Expected:    expected.String(),
		}})
	}
	return planned
}

// applyGrants writes the grants of one space. They come in the order the check
// put them in, parents before children, so the storage is never left granting
// more below a path than on it.
func (j *ShallowJob) applyGrants(ctx context.Context, log *zerolog.Logger, report *ShallowReport, planned []plannedGrant, traces map[string]string) {
	for _, p := range planned {
		w := p.entry
		traceID := traces[w.ShareID]

		if !j.DryRun {
			if err := j.write(trace.Set(ctx, traceID), p.share, w.Action); err != nil {
				report.Failed++
				log.Error().Err(err).
					Str("event", EventShallowFail).
					Str("share", w.ShareID).
					Str("path", w.Path).
					Str("grantee", w.Grantee).
					Str("grantee_type", w.GranteeType).
					Str("traceid", traceID).
					Msg("reconciliation: writing the grant failed")
				continue
			}
		}

		report.Written = append(report.Written, w)
		log.Info().
			Str("event", EventShallowGrant).
			Str("share", w.ShareID).
			Str("action", w.Action.String()).
			Str("path", w.Path).
			Str("storage_id", w.ResourceID.GetStorageId()).
			Str("opaque_id", w.ResourceID.GetOpaqueId()).
			Str("grantee", w.Grantee).
			Str("grantee_type", w.GranteeType).
			Str("observed", w.Observed).
			Str("expected", w.Expected).
			Bool("dry_run", j.DryRun).
			Str("traceid", traceID).
			Msg("reconciliation: grant written")
	}
}

// applyRemovals removes the redundant shares of one space.
func (j *ShallowJob) applyRemovals(ctx context.Context, log *zerolog.Logger, report *ShallowReport, redundant []RemovedShare, traces map[string]string) {
	for _, r := range redundant {
		traceID := traces[r.ShareID]
		if !j.DryRun {
			ref := &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: r.ShareID}},
			}
			if err := j.ShareStore.Unshare(trace.Set(ctx, traceID), ref); err != nil {
				report.Failed++
				log.Error().Err(err).
					Str("event", EventShallowFail).
					Str("share", r.ShareID).
					Str("path", r.Path).
					Str("grantee", r.Grantee).
					Str("grantee_type", r.GranteeType).
					Str("traceid", traceID).
					Msg("reconciliation: removing the redundant share failed")
				continue
			}
		}

		report.Removed = append(report.Removed, r)
		log.Info().
			Str("event", EventShallowRemove).
			Str("share", r.ShareID).
			Str("path", r.Path).
			Str("storage_id", r.ResourceID.GetStorageId()).
			Str("opaque_id", r.ResourceID.GetOpaqueId()).
			Str("grantee", r.Grantee).
			Str("grantee_type", r.GranteeType).
			Str("level", r.Level).
			Str("reason", string(r.Reason)).
			Str("ancestor", r.AncestorID).
			Str("ancestor_path", r.AncestorPath).
			Str("ancestor_role", r.AncestorRole).
			Bool("dry_run", j.DryRun).
			Str("traceid", traceID).
			Msg("reconciliation: redundant share removed")
	}
}

// appeared lists the shares of a space again and returns the id of the first one
// the run has not seen, empty when there is none. A share created while the run
// was checking is not in what the check compared, and it may be exactly what an
// ancestor of it needs, or what stops a share the run wants to remove from being
// redundant, so the caller leaves the whole space to the next run.
func (j *ShallowJob) appeared(ctx context.Context, spaceID string, seen map[string]struct{}) (string, error) {
	shares, err := j.ShareStore.ListShares(ctx, []*collaboration.Filter{revashare.SpaceIDFilter(spaceID)})
	if err != nil {
		return "", errors.Wrapf(err, "reconciliation: listing the shares of space %q again", spaceID)
	}
	for _, s := range shares {
		id := s.GetId().GetOpaqueId()
		if _, ok := seen[id]; !ok {
			return id, nil
		}
	}
	return "", nil
}

// shadowed counts a share that gets no entry of its own because a share above
// it shadows it, and returns the removal that calls for. Inherited means the
// share above grants the same recipient the same permissions, so the entry is
// already there; anything else means the database holds a share the hierarchy
// check would have rejected. Either way the share is one that creating the
// share above it would have deleted, so it is removed rather than carried
// along.
//
// The second result is false when the check failed for any other reason, which
// leaves the share alone: only a named ancestor makes a share redundant.
func (j *ShallowJob) shadowed(log *zerolog.Logger, report *ShallowReport, rs sharehierarchy.ResolvedShare, traceID string, err error) (RemovedShare, bool) {
	level := sharehierarchy.PermLevelFromCS3(rs.Share.GetPermissions().GetPermissions())
	sharee, granteeType := sharehierarchy.ShareeInfo(rs.Share.GetGrantee())

	var conflict *sharehierarchy.HierarchyConflictError
	if !errors.As(err, &conflict) || len(conflict.ConflictingShares) == 0 {
		report.Skipped++
		log.Error().Err(err).
			Str("event", EventShallowSkip).
			Str("share", rs.Share.GetId().GetOpaqueId()).
			Str("path", rs.Path).
			Str("traceid", traceID).
			Msg("reconciliation: hierarchy check failed, share left untouched")
		return RemovedShare{}, false
	}

	ancestor := conflict.ConflictingShares[0]
	reason := ReasonShadowedByAncestor
	if ancestor.PermissionType == level.RoleID() {
		reason = ReasonInherited
		report.Covered++
	} else {
		report.Conflicting++
	}

	return RemovedShare{
		ShareID:      rs.Share.GetId().GetOpaqueId(),
		Path:         rs.Path,
		ResourceID:   rs.Share.GetResourceId(),
		Grantee:      sharee,
		GranteeType:  granteeType,
		Level:        level.String(),
		Reason:       reason,
		AncestorID:   ancestor.ID,
		AncestorPath: ancestor.Path,
		AncestorRole: ancestor.PermissionType,
	}, true
}

// gateway returns the client the run resolves paths and recipients with: the
// injected one when set, otherwise the one the service registry resolves right
// now.
func (j *ShallowJob) gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	if j.Gateway != nil {
		return j.Gateway, nil
	}
	return service.Gateway(ctx)
}

// resolve pairs a share with the current path of its resource and the verified
// identity of its recipient. It fails when either cannot be resolved, which
// leaves the share alone rather than writing an entry built on a guess.
func (j *ShallowJob) resolve(ctx context.Context, gw gateway.GatewayAPIClient, grantees map[string]*provider.Grantee, s *collaboration.Share) (sharehierarchy.ResolvedShare, error) {
	res, err := gw.GetPath(ctx, &provider.GetPathRequest{ResourceId: s.GetResourceId()})
	if err != nil {
		return sharehierarchy.ResolvedShare{}, errors.Wrap(err, "reconciliation: get path")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return sharehierarchy.ResolvedShare{}, errors.Errorf("reconciliation: get path: %s: %s", code, res.GetStatus().GetMessage())
	}

	grantee, err := j.grantee(ctx, gw, grantees, s.GetGrantee())
	if err != nil {
		return sharehierarchy.ResolvedShare{}, err
	}

	share := proto.Clone(s).(*collaboration.Share)
	share.Grantee = grantee
	return sharehierarchy.ResolvedShare{Share: share, Path: res.GetPath()}, nil
}

// grantee verifies the recipient of a share. A user grantee is resolved through
// the gateway rather than taken as it comes, because its user type is what
// tells the storage driver where the entry goes: a lightweight account is not
// written to the native ACLs. The share manager falls back to a primary account
// when that lookup fails, which here is an error instead: guessing the type
// would write an external account into the native ACLs.
// A name that resolves is kept in grantees for the rest of the run. A name that
// does not is not kept, so the next share for it is looked up again.
func (j *ShallowJob) grantee(ctx context.Context, gw gateway.GatewayAPIClient, grantees map[string]*provider.Grantee, g *provider.Grantee) (*provider.Grantee, error) {
	if g.GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP {
		return g, nil
	}

	username := g.GetUserId().GetOpaqueId()
	if verified, ok := grantees[username]; ok {
		return verified, nil
	}

	res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username", Value: username, SkipFetchingUserGroups: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: get user")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return nil, errors.Errorf("reconciliation: get user %q: %s: %s", username, code, res.GetStatus().GetMessage())
	}

	verified := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: res.GetUser().GetId()},
	}
	grantees[username] = verified
	return verified, nil
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

// Periodic wraps a full run as an rjobs.Periodic. It runs on the leader because
// it mutates the storage, and skips a fire if the previous run is still going.
func (j *ShallowJob) Periodic(schedule string) rjobs.Periodic {
	return rjobs.Periodic{
		Name:       ShallowScheduleName,
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

// ShallowParams are the parameters of one on-demand run.
type ShallowParams struct {
	// SpaceID limits the run to one space. Empty covers every space.
	SpaceID string `mapstructure:"space"`
}

// OnDemand has the shape of rjobs.NewJob, so it is what gets registered with
// rjobs.RegisterOnDemand. The job is already built, so the configuration
// section the runner passes is not read.
func (j *ShallowJob) OnDemand(context.Context, map[string]any) (rjobs.Job, error) {
	return &shallowRun{job: j}, nil
}

type shallowRun struct {
	job *ShallowJob
}

// Run performs one run and returns its totals as the run's result.
func (r *shallowRun) Run(ctx context.Context, p rjobs.Params) (rjobs.Params, error) {
	var params ShallowParams
	// a parameter that is not read is an error: a mistyped "space" would
	// otherwise silently widen the run from one space to every space.
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      &params,
		ErrorUnused: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: building the parameter decoder")
	}
	if err := dec.Decode(map[string]any(p)); err != nil {
		return nil, errors.Wrap(err, "reconciliation: decoding the shallow parameters")
	}

	report, err := r.job.run(ctx, params.SpaceID)
	if err != nil {
		return nil, err
	}
	return rjobs.Params{
		"run":         report.RunID,
		"space":       report.SpaceID,
		"dry_run":     report.DryRun,
		"checked":     report.Checked,
		"covered":     report.Covered,
		"conflicting": report.Conflicting,
		"written":     len(report.Written),
		"removed":     len(report.Removed),
		"skipped":     report.Skipped,
		// the spaces left for the next run, named so that a run can be retried on
		// exactly the ones it held back.
		"skipped_spaces": report.SkippedSpaces,
		"failed":         report.Failed,
	}, nil
}
