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
	"github.com/pkg/errors"
)

// OrphanJobName is the stable identity of the orphan job.
const OrphanJobName = "reconciliation.orphans"

// OrphanReason says why a share was marked orphaned.
type OrphanReason string

const (
	// ReasonResourceMissing means the shared resource no longer exists. Its
	// storage space being gone shows up here too, since the resource Stat then
	// fails.
	ReasonResourceMissing OrphanReason = "resource-missing"
	// ReasonRecipientMissing means the user or group the share is for no longer
	// exists.
	ReasonRecipientMissing OrphanReason = "recipient-missing"
)

// ShareStore is the subset of the share manager the orphan job needs.
// *sql.ShareMgr satisfies it.
type ShareStore interface {
	// ListModelShares returns the shares matching the filters. Pass a nil user
	// to list across all owners and hideOrphans=true to skip already-orphaned
	// shares.
	ListModelShares(u *userpb.User, filters []*collaboration.Filter, hideOrphans bool) ([]model.Share, error)
	// MarkAsOrphaned flags the referenced share as orphaned.
	MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error
}

// OrphanJob marks shares whose resource or recipient is gone as orphaned. It is
// idempotent: a share already orphaned is skipped by the hideOrphans filter, and
// re-running never marks a valid share.
type OrphanJob struct {
	// Shares is the share store to scan and mutate.
	Shares ShareStore
	// Gateway resolves resource and recipient existence.
	Gateway gateway.GatewayAPIClient
	// DryRun, when set, reports what would be orphaned without mutating.
	DryRun bool
}

// OrphanedShare records one share the job orphaned (or, in dry-run, would have).
type OrphanedShare struct {
	// ShareID is the CS3 opaque id of the share.
	ShareID string
	// Reason is why it was orphaned.
	Reason OrphanReason
	// ResourceID is the shared resource, for logging.
	ResourceID *provider.ResourceId
	// ShareWith is the recipient (username, group name or external id), for
	// logging.
	ShareWith string
}

// OrphanReport summarises a run.
type OrphanReport struct {
	// Checked is the number of non-orphan shares examined.
	Checked int
	// Skipped is the number of shares left undecided because a lookup failed.
	Skipped int
	// Orphaned lists the shares marked (or, in dry-run, that would be marked).
	Orphaned []OrphanedShare
	// DryRun reports whether the run was a simulation.
	DryRun bool
}

// Run scans the share store and orphans shares whose resource or recipient is
// gone. A per-share lookup failure is logged and the share is skipped, never
// orphaned, so a flaky gateway can never produce a false orphan. The run itself
// only fails if the shares cannot be listed at all.
func (j *OrphanJob) Run(ctx context.Context) (OrphanReport, error) {
	log := appctx.GetLogger(ctx)

	shares, err := j.Shares.ListModelShares(nil, nil, true)
	if err != nil {
		return OrphanReport{}, errors.Wrap(err, "reconciliation: listing shares")
	}

	report := OrphanReport{DryRun: j.DryRun}
	for i := range shares {
		s := &shares[i]
		report.Checked++

		reason, orphaned, err := j.classify(ctx, s)
		if err != nil {
			report.Skipped++
			log.Error().Err(err).Uint("share_id", s.Id).Msg("reconciliation: existence check failed, skipping share")
			continue
		}
		if !orphaned {
			continue
		}

		rec := OrphanedShare{
			ShareID:    strconv.FormatUint(uint64(s.Id), 10),
			Reason:     reason,
			ResourceID: &provider.ResourceId{StorageId: s.Instance, OpaqueId: s.Inode},
			ShareWith:  s.ShareWith,
		}
		report.Orphaned = append(report.Orphaned, rec)

		if j.DryRun {
			log.Info().Str("share_id", rec.ShareID).Str("reason", string(reason)).Msg("reconciliation: would mark share orphaned (dry_run)")
			continue
		}

		if err := j.Shares.MarkAsOrphaned(ctx, shareRefByID(s.Id)); err != nil {
			log.Error().Err(err).Str("share_id", rec.ShareID).Msg("reconciliation: marking share orphaned failed")
			continue
		}
		log.Info().Str("share_id", rec.ShareID).Str("reason", string(reason)).Msg("reconciliation: marked share orphaned")
	}

	return report, nil
}

// classify decides whether a share is orphaned and why, using gateway lookups.
// It returns an error only on a lookup failure, which the caller treats as
// "undecided", not as absence.
func (j *OrphanJob) classify(ctx context.Context, s *model.Share) (OrphanReason, bool, error) {
	statRes, err := j.Gateway.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: s.Instance, OpaqueId: s.Inode}},
	})
	if err != nil {
		return "", false, errors.Wrap(err, "reconciliation: stat")
	}
	if exists, err := existsFromStatus(statRes.GetStatus()); err != nil {
		return "", false, err
	} else if !exists {
		return ReasonResourceMissing, true, nil
	}

	var st *rpc.Status
	if s.SharedWithIsGroup {
		res, err := j.Gateway.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
			Claim: "group_name", Value: s.ShareWith, SkipFetchingMembers: true,
		})
		if err != nil {
			return "", false, errors.Wrap(err, "reconciliation: get group")
		}
		st = res.GetStatus()
	} else {
		res, err := j.Gateway.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim: "username", Value: s.ShareWith, SkipFetchingUserGroups: true,
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

// shareRefByID builds a share reference addressing a share by its numeric id.
func shareRefByID(id uint) *collaboration.ShareReference {
	return &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: &collaboration.ShareId{OpaqueId: strconv.FormatUint(uint64(id), 10)},
		},
	}
}

// Periodic wraps the job as an rjobs.Periodic. It runs on the leader because it
// mutates shared database state, and skips a fire if the previous run is still
// going.
func (j *OrphanJob) Periodic(schedule string) rjobs.Periodic {
	return rjobs.Periodic{
		Name:     OrphanJobName,
		Schedule: schedule,
		Scope:    rjobs.ScopeLeader,
		Overlap:  rjobs.Skip,
		Run: func(ctx context.Context) error {
			_, err := j.Run(ctx)
			return err
		},
	}
}
