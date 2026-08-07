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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// OrphanJobName is the stable identity of the orphan job.
const OrphanJobName = "reconciliation.orphans"

// The events the orphan job logs, see the package comment.
const (
	// EventOrphanStart opens a run.
	EventOrphanStart = OrphanJobName + ".start"
	// EventOrphanMark reports one item marked orphaned, or, in dry-run, one
	// that would have been. It carries every field needed to undo the change.
	EventOrphanMark = OrphanJobName + ".mark"
	// EventOrphanSkip reports one item left untouched because a lookup failed.
	EventOrphanSkip = OrphanJobName + ".skip"
	// EventOrphanFail reports one item that was classified as orphaned but
	// could not be marked.
	EventOrphanFail = OrphanJobName + ".fail"
	// EventOrphanEnd closes a run with its totals.
	EventOrphanEnd = OrphanJobName + ".end"
)

// Kind is the type of share-like object an entry refers to.
type Kind string

const (
	// KindShare is a share between users or groups.
	KindShare Kind = "share"
	// KindPublicLink is a public link.
	KindPublicLink Kind = "publiclink"
)

// OrphanReason says why an item was marked orphaned.
type OrphanReason string

const (
	// ReasonResourceMissing means the shared resource no longer exists.
	//
	// A resource in the recycle bin does not count as missing and is never
	// orphaned: a delete has to stay reversible, so a restored resource must
	// find its shares intact.
	ReasonResourceMissing OrphanReason = "resource-missing"
	// ReasonRecipientMissing means the user or group the share is for no longer
	// exists.
	ReasonRecipientMissing OrphanReason = "recipient-missing"
)

// ShareStore is what the orphan job needs from a share manager: the CS3 listing
// every manager implements, plus marking, which the CS3 API has no call for. A
// manager that cannot mark cannot be reconciled. The listing is expected to
// cover every owner and to leave out the already-orphaned.
type ShareStore interface {
	ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error)
	MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error
	// Unshare removes the referenced share. The row is soft deleted, so a
	// removal can be undone in the database.
	Unshare(ctx context.Context, ref *collaboration.ShareReference) error
}

// PublicLinkStore is the same for a public share manager.
type PublicLinkStore interface {
	ListPublicShares(ctx context.Context, u *userpb.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo, sign bool) ([]*link.PublicShare, error)
	MarkAsOrphaned(ctx context.Context, ref *link.PublicShareReference) error
}

// OrphanJob marks shares and public links whose resource or recipient is gone
// as orphaned. It is idempotent: an item already orphaned is skipped by the
// hideOrphans filter, and re-running never marks a valid one.
type OrphanJob struct {
	// Shares is the share store to scan and mutate. A nil store leaves shares
	// unscanned.
	Shares ShareStore
	// Links is the public link store to scan and mutate. A nil store leaves
	// public links unscanned.
	Links PublicLinkStore
	// Gateway resolves resource and recipient existence. When nil the job
	// resolves the gateway through the service registry at the start of every
	// run, which is what the serverless service relies on: the registry may not
	// hold a gateway yet when the job is registered at startup.
	Gateway gateway.GatewayAPIClient
	// Auth puts the identity the job acts as on the run context. The jobs
	// runner hands a run a bare context and the gateway rejects a call without
	// a token, so every run needs one. A nil Auth leaves the context alone.
	Auth func(ctx context.Context) (context.Context, error)
	// Log is the job's own log, see OpenLog. When nil the job falls back to the
	// logger in the run context.
	Log *zerolog.Logger
	// DryRun, when set, reports what would be orphaned without mutating.
	DryRun bool
	// RunOnStart, when set, fires the job once as soon as the runner starts.
	RunOnStart bool
}

// entry is one share-like item to check. Shares and public links are flattened
// into it so both go through the same classification: a share and a public
// share are unrelated CS3 types, so there is no common type to range over.
type entry struct {
	kind Kind
	// id is the CS3 opaque id of the share or link.
	id         string
	resourceID *provider.ResourceId
	owner      string
	// shareWith is the recipient. It is empty for public links, which have no
	// grantee and are therefore only checked against their resource.
	shareWith string
	isGroup   bool
}

// OrphanedItem records one item the job orphaned (or, in dry-run, would have).
type OrphanedItem struct {
	// Kind is the type of item.
	Kind Kind
	// ID is the CS3 opaque id of the share or link.
	ID string
	// Reason is why it was orphaned.
	Reason OrphanReason
	// ResourceID is the shared resource, for logging.
	ResourceID *provider.ResourceId
	// ShareWith is the recipient (username, group name or external id), empty
	// for public links.
	ShareWith string
}

// OrphanReport summarises a run.
type OrphanReport struct {
	// RunID identifies the run. Every log line it wrote carries it under "run".
	RunID string
	// Checked is the number of non-orphan items examined.
	Checked int
	// Skipped is the number of items left undecided because a lookup failed.
	Skipped int
	// Failed is the number of items classified as orphaned that could not be
	// marked.
	Failed int
	// Orphaned lists the items marked (or, in dry-run, that would be marked).
	Orphaned []OrphanedItem
	// DryRun reports whether the run was a simulation.
	DryRun bool
}

// Run scans both stores and orphans the items whose resource or recipient is
// gone. A per-item lookup failure is logged and the item is skipped, never
// orphaned, so a flaky gateway can never produce a false orphan. The run itself
// only fails if the stores cannot be listed at all.
//
// Every decision is logged with the identifiers needed to revert it. Items that
// pass the check are logged at debug level only, since there is normally one
// per row in the database.
func (j *OrphanJob) Run(ctx context.Context) (OrphanReport, error) {
	base := j.Log
	if base == nil {
		base = appctx.GetLogger(ctx)
	}
	// every line of a run carries the same "run" field, so one run can be
	// picked out of a log holding many, and the "job" field, since jobs can
	// share a log file.
	runID := uuid.New().String()
	l := base.With().Str("job", OrphanJobName).Str("run", runID).Logger()
	log := &l

	if j.Auth != nil {
		var err error
		if ctx, err = j.Auth(ctx); err != nil {
			return OrphanReport{}, err
		}
	}

	gw, err := j.gateway(ctx)
	if err != nil {
		return OrphanReport{}, err
	}

	entries, err := j.entries(ctx)
	if err != nil {
		return OrphanReport{}, err
	}

	log.Info().
		Str("event", EventOrphanStart).
		Bool("dry_run", j.DryRun).
		Int("candidates", len(entries)).
		Msg("reconciliation: run started")

	report := OrphanReport{RunID: runID, DryRun: j.DryRun}
	for _, e := range entries {
		report.Checked++

		reason, orphaned, err := j.classify(ctx, gw, e)
		if err != nil {
			report.Skipped++
			log.Error().Err(err).
				Str("event", EventOrphanSkip).
				Str("kind", string(e.kind)).
				Str("id", e.id).
				Msg("reconciliation: existence check failed, item left untouched")
			continue
		}
		if !orphaned {
			log.Debug().
				Str("kind", string(e.kind)).
				Str("id", e.id).
				Msg("reconciliation: item is valid")
			continue
		}

		if !j.DryRun {
			if err := j.mark(ctx, e); err != nil {
				report.Failed++
				log.Error().Err(err).
					Str("event", EventOrphanFail).
					Str("kind", string(e.kind)).
					Str("id", e.id).
					Msg("reconciliation: marking item orphaned failed")
				continue
			}
		}

		report.Orphaned = append(report.Orphaned, OrphanedItem{
			Kind:       e.kind,
			ID:         e.id,
			Reason:     reason,
			ResourceID: e.resourceID,
			ShareWith:  e.shareWith,
		})
		log.Info().
			Str("event", EventOrphanMark).
			Str("kind", string(e.kind)).
			Str("id", e.id).
			Str("reason", string(reason)).
			Str("storage_id", e.resourceID.GetStorageId()).
			Str("opaque_id", e.resourceID.GetOpaqueId()).
			Str("owner", e.owner).
			Str("share_with", e.shareWith).
			Bool("dry_run", j.DryRun).
			Msg("reconciliation: item marked orphaned")
	}

	log.Info().
		Str("event", EventOrphanEnd).
		Bool("dry_run", j.DryRun).
		Int("checked", report.Checked).
		Int("orphaned", len(report.Orphaned)).
		Int("skipped", report.Skipped).
		Int("failed", report.Failed).
		Msg("reconciliation: run finished")

	return report, nil
}

// entries lists the configured stores and flattens them into a single list to
// check.
func (j *OrphanJob) entries(ctx context.Context) ([]entry, error) {
	var entries []entry

	if j.Shares != nil {
		shares, err := j.Shares.ListShares(ctx, nil)
		if err != nil {
			return nil, errors.Wrap(err, "reconciliation: listing shares")
		}
		for _, s := range shares {
			e := entry{
				kind:       KindShare,
				id:         s.GetId().GetOpaqueId(),
				resourceID: s.GetResourceId(),
				owner:      s.GetOwner().GetOpaqueId(),
			}
			if g := s.GetGrantee(); g.GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP {
				e.isGroup = true
				e.shareWith = g.GetGroupId().GetOpaqueId()
			} else {
				e.shareWith = g.GetUserId().GetOpaqueId()
			}
			entries = append(entries, e)
		}
	}

	if j.Links != nil {
		links, err := j.Links.ListPublicShares(ctx, nil, nil, nil, false)
		if err != nil {
			return nil, errors.Wrap(err, "reconciliation: listing public links")
		}
		for _, l := range links {
			entries = append(entries, entry{
				kind:       KindPublicLink,
				id:         l.GetId().GetOpaqueId(),
				resourceID: l.GetResourceId(),
				owner:      l.GetOwner().GetOpaqueId(),
			})
		}
	}

	return entries, nil
}

// gateway returns the client the run checks existence with: the injected one
// when set, otherwise the one the service registry resolves right now.
func (j *OrphanJob) gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	if j.Gateway != nil {
		return j.Gateway, nil
	}
	return service.Gateway(ctx)
}

// classify decides whether an entry is orphaned and why, using gateway lookups.
// It returns an error only on a lookup failure, which the caller treats as
// "undecided", not as absence.
func (j *OrphanJob) classify(ctx context.Context, gw gateway.GatewayAPIClient, e entry) (OrphanReason, bool, error) {
	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: e.resourceID},
	})
	if err != nil {
		return "", false, errors.Wrap(err, "reconciliation: stat")
	}
	if exists, err := existsFromStatus(statRes.GetStatus()); err != nil {
		return "", false, err
	} else if !exists {
		return ReasonResourceMissing, true, nil
	}

	// public links have no grantee, so the resource is all there is to check.
	if e.shareWith == "" {
		return "", false, nil
	}

	var st *rpc.Status
	if e.isGroup {
		res, err := gw.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
			Claim: "group_name", Value: e.shareWith, SkipFetchingMembers: true,
		})
		if err != nil {
			return "", false, errors.Wrap(err, "reconciliation: get group")
		}
		st = res.GetStatus()
	} else {
		res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim: "username", Value: e.shareWith, SkipFetchingUserGroups: true,
		})
		if err != nil {
			return "", false, errors.Wrap(err, "reconciliation: get user")
		}
		st = res.GetStatus()
	}
	if exists, err := existsFromStatus(st); err != nil {
		return "", false, err
	} else if !exists {
		return ReasonRecipientMissing, true, nil
	}

	return "", false, nil
}

// mark flags the entry as orphaned in the store it came from.
func (j *OrphanJob) mark(ctx context.Context, e entry) error {
	switch e.kind {
	case KindShare:
		return j.Shares.MarkAsOrphaned(ctx, &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{OpaqueId: e.id},
			},
		})
	case KindPublicLink:
		return j.Links.MarkAsOrphaned(ctx, &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{OpaqueId: e.id},
			},
		})
	default:
		return errors.Errorf("reconciliation: unknown kind %q", e.kind)
	}
}

// existsFromStatus maps a CS3 status to existence: OK is present, NOT_FOUND is a
// confirmed absence, anything else is a real error (undecided, not absent).
func existsFromStatus(s *rpc.Status) (bool, error) {
	switch s.GetCode() {
	case rpc.Code_CODE_OK:
		return true, nil
	case rpc.Code_CODE_NOT_FOUND:
		return false, nil
	default:
		return false, errors.Errorf("reconciliation: unexpected status %s: %s", s.GetCode(), s.GetMessage())
	}
}

// Periodic wraps the job as an rjobs.Periodic. It runs on the leader because it
// mutates shared database state, and skips a fire if the previous run is still
// going.
func (j *OrphanJob) Periodic(schedule string) rjobs.Periodic {
	return rjobs.Periodic{
		Name:       OrphanJobName,
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
