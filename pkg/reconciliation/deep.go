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
	"cmp"
	"context"
	"slices"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/reconciliation/nsdump"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/acl"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// TODO(jgeens):
// - check what to do with version folders

const DeepJobName = "reconciliation.deep"

const (
	EventDeepStart     = DeepJobName + ".start"
	EventDeepChange    = DeepJobName + ".change"
	EventDeepSkipSpace = DeepJobName + ".skipspace"
	EventDeepFail      = DeepJobName + ".fail"
	EventDeepEnd       = DeepJobName + ".end"
)

type DeepJob struct {
	ShareStore ShareStore
	// Gateway is resolved through the service registry per run when nil.
	Gateway gateway.GatewayAPIClient
	// Auth returns ctx carrying the token the run calls the gateway with.
	Auth func(ctx context.Context) (context.Context, error)
	// Instance returns the EOS instance of a storage id. The storage id of a
	// resource is a routing alias (eoshome-a), but the namespace dump and the
	// EOS client need the instance itself (eoshome-i01).
	Instance func(ctx context.Context, storageID string) (string, error)
	// Project returns the project space with the given space id, or nil when
	// the space is not a project.
	Project func(ctx context.Context, spaceID string) (*provider.StorageSpace, error)
	// The changes are calculated per node, and the grant calls of the storage
	// provider always apply an ACL recursively, so we talk to EOS ourselves.
	// Eos holds the settings all instances share: each space gets its own
	// client, pointed at the MGM of its instance.
	Eos    eos.Config
	Log    *zerolog.Logger
	DryRun bool
}

type ChangeSet []*Change

type Change struct {
	Path   string
	Action ActionKind
	ACL    *acl.Entry
}

type RunParameters struct {
	SpaceID       string
	OptionalACLs  []*acl.Entry
	MandatoryACLs []*acl.Entry
}

type ShareWithPath struct {
	*collaborationv1beta1.Share
	Path string
}

func (s *ShareWithPath) toTreeNode() *ACLNode {
	return &ACLNode{
		Path:          s.Path,
		MandatoryACLs: []*acl.Entry{shareToACL(s.Share)},
	}
}

// DeepConfig configures the deep job. The EOS keys are the ones of the eos
// storage driver, of which we only need the client
type DeepConfig struct {
	Config `mapstructure:",squash"`
	Eos    eos.Config `mapstructure:"eos"`
}

// DeepReport summarises a run.
type DeepReport struct {
	// RunID identifies the run. Every log line it wrote carries it under "run".
	RunID string
	// SpaceID is the space the run was limited to, empty for a full run.
	SpaceID string
	// Spaces is the number of spaces that were compared.
	Spaces  int
	Added   int
	Updated int
	Deleted int
	// SkippedSpaces lists the spaces nothing was applied to, because they
	// could not be compared or because a share appeared while the run was
	// comparing them.
	SkippedSpaces []string
	// Failed is the number of spaces where applying the changes stopped at a
	// failure. The changes after that one are not on the storage.
	Failed int
	DryRun bool
}

// Run reconciles every space that holds a share, one space at a time. A space
// that cannot be compared is logged and skipped, and the run goes on with the
// next one. A cancelled run stops at the next space and gives back the
// cancellation, with the report of what it did get through.
func (j *DeepJob) Run(ctx context.Context) (DeepReport, error) {
	return j.run(ctx, "", rjobs.NopReporter{})
}

// RunSpace is Run over a single space.
func (j *DeepJob) RunSpace(ctx context.Context, spaceID string) (DeepReport, error) {
	if spaceID == "" {
		return DeepReport{}, errors.New("reconciliation: no space given")
	}
	return j.run(ctx, spaceID, rjobs.NopReporter{})
}

// run does the work of both, over every space when spaceID is empty.
func (j *DeepJob) run(ctx context.Context, spaceID string, rep rjobs.Reporter) (DeepReport, error) {
	base := j.Log
	if base == nil {
		base = appctx.GetLogger(ctx)
	}
	runID := uuid.New().String()
	log := base.With().Str("job", DeepJobName).Str("run", runID).Logger()

	if j.Auth != nil {
		var err error
		if ctx, err = j.Auth(ctx); err != nil {
			return DeepReport{}, err
		}
	}

	// after Auth, so the resolved client is used with a context carrying the
	// job's token.
	gw, err := j.gateway(ctx)
	if err != nil {
		return DeepReport{}, err
	}

	spaceIDs := []string{spaceID}
	if spaceID == "" {
		if spaceIDs, err = j.ShareStore.ListShareSpaces(ctx); err != nil {
			return DeepReport{}, errors.Wrap(err, "reconciliation: listing the spaces that hold a share")
		}
		// a fixed order, so two runs over the same state read the same way.
		slices.Sort(spaceIDs)
	}

	log.Info().
		Str("event", EventDeepStart).
		Bool("dry_run", j.DryRun).
		Int("spaces", len(spaceIDs)).
		Msg("reconciliation: run started")

	report := DeepReport{RunID: runID, SpaceID: spaceID, DryRun: j.DryRun}
	var cancelled error
	for i, space := range spaceIDs {
		if cancelled = ctx.Err(); cancelled != nil {
			break
		}
		report.progress(rep, i, len(spaceIDs))

		spaceLog := log.With().Str("space", space).Logger()
		err := j.reconcileSpace(ctx, &spaceLog, &report, gw, space)
		if err == nil {
			continue
		}
		// one space means nothing else to carry on with, so the failure is the
		// run's rather than an empty success.
		if len(spaceIDs) == 1 {
			return DeepReport{}, err
		}
		report.SkippedSpaces = append(report.SkippedSpaces, space)
		spaceLog.Error().Err(err).
			Str("event", EventDeepSkipSpace).
			Bool("dry_run", j.DryRun).
			Msg("reconciliation: space could not be compared, left untouched")
	}
	report.progress(rep, len(spaceIDs), len(spaceIDs))

	log.Info().
		Str("event", EventDeepEnd).
		Bool("dry_run", j.DryRun).
		Int("spaces", report.Spaces).
		Int("added", report.Added).
		Int("updated", report.Updated).
		Int("deleted", report.Deleted).
		Int("skipped_spaces", len(report.SkippedSpaces)).
		Int("failed", report.Failed).
		Msg("reconciliation: run finished")

	return report, cancelled
}

// reconcileSpace compares one space against its shares and applies the
// differences. The error is the space not being comparable at all; a failure
// while applying is logged and counted instead, since part of the changes may
// already be on the storage.
func (j *DeepJob) reconcileSpace(ctx context.Context, log *zerolog.Logger, report *DeepReport, gw gateway.GatewayAPIClient, spaceID string) error {
	root, err := spaces.DecodeSpaceID(spaceID)
	if err != nil {
		return errors.Wrapf(err, "reconciliation: decoding space %q", spaceID)
	}

	statRes, err := gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: root}})
	if err != nil {
		return errors.Wrapf(err, "reconciliation: stat of the space root %s", root)
	}
	if code := statRes.GetStatus().GetCode(); code != rpcv1beta1.Code_CODE_OK {
		return errors.Errorf("reconciliation: stat of the space root %s: %s: %s", root, code, statRes.GetStatus().GetMessage())
	}

	instance, err := j.Instance(ctx, statRes.GetInfo().GetId().GetStorageId())
	if err != nil {
		return err
	}

	p := RunParameters{SpaceID: spaceID}
	project, err := j.Project(ctx, spaceID)
	if err != nil {
		return errors.Wrapf(err, "reconciliation: looking up the project of space %q", spaceID)
	}
	if project != nil {
		for _, role := range project.GetRoles() {
			p.MandatoryACLs = append(p.MandatoryACLs, roleToAcls(role)...)
		}
	} else {
		p.MandatoryACLs = append(p.MandatoryACLs, &acl.Entry{
			Type:        acl.TypeUser,
			Qualifier:   statRes.GetInfo().GetOwner().GetOpaqueId(),
			Permissions: "rwx",
		})
	}

	namespaceDumper := &nsdump.EOSMemoryNSInspect{}
	if err := namespaceDumper.Setup(nsdump.EOSMemoryNSInspectConfig{Instance: instance}); err != nil {
		return err
	}

	cs, seen, err := j.runAnalysis(ctx, gw, p, namespaceDumper)
	if err != nil {
		return err
	}
	report.Spaces++

	if j.DryRun || len(cs) == 0 {
		j.writeJournal(log, report, cs, nil)
		return nil
	}

	// a share created while the run was comparing is not in the tree, so the
	// change set could delete the entry it needs.
	fresh, err := appeared(ctx, j.ShareStore, spaceID, seen)
	if err != nil {
		return err
	}
	if fresh != "" {
		report.SkippedSpaces = append(report.SkippedSpaces, spaceID)
		log.Warn().
			Str("event", EventDeepSkipSpace).
			Str("new_share", fresh).
			Int("changes", len(cs)).
			Msg("reconciliation: a share was created while the run was comparing, space left untouched")
		return nil
	}

	conf := j.Eos
	conf.MasterURL = "root://" + instance
	conf.GrpcURI = instance + ":50051"
	client, err := eos.NewEOSClient(ctx, &conf)
	if err != nil {
		return errors.Wrapf(err, "reconciliation: building the eos client of %s", instance)
	}

	// Analysis happened without errors, so we can apply the changes and write to a journal
	applyErr := j.applyChangeSet(ctx, client, cs)
	j.writeJournal(log, report, cs, applyErr)
	return nil
}

// runAnalysis also returns the ids of the shares it compared, so the caller
// can tell a share created after the listing.
func (j *DeepJob) runAnalysis(ctx context.Context, gw gateway.GatewayAPIClient, p RunParameters, nsdumper nsdump.NSDumpClient) (ChangeSet, map[string]struct{}, error) {
	path, err := spaces.DecodeSpaceID(p.SpaceID)
	if err != nil {
		return nil, nil, err
	}

	// First, construct a tree which contains the "ideal" state
	// For this, we:
	// 1. Get all the shares in the space
	// 2. Resolve all their paths
	// 3. Sort them (this makes the tree insertion much faster)
	// 4. Construct the tree
	shares, err := j.ShareStore.ListShares(ctx, []*collaborationv1beta1.Filter{
		{
			Type: collaborationv1beta1.Filter_TYPE_SPACE_ID,
			Term: &collaborationv1beta1.Filter_SpaceId{
				SpaceId: p.SpaceID,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	// Resolve the paths
	seen := make(map[string]struct{}, len(shares))
	now := time.Now()
	var sharesWithPaths = make([]*ShareWithPath, 0, len(shares))
	for _, s := range shares {
		seen[s.GetId().GetOpaqueId()] = struct{}{}
		// an expired share has no entry, as in the gateway: the expired job
		// removes it, and we must not put it back.
		if isExpired(s, now) {
			continue
		}
		if sp, ok := getPath(ctx, gw, s.ResourceId); ok {
			sharesWithPaths = append(sharesWithPaths, &ShareWithPath{
				Share: s,
				Path:  sp,
			})
		}
	}

	// Sort the shares,
	slices.SortFunc(sharesWithPaths, func(a, b *ShareWithPath) int {
		return cmp.Compare(a.Path, b.Path)
	})

	// Finally, construct the tree. The space root goes first: its entries are
	// set recursively, so every node of the space needs them.
	tree := NewACLTree()
	tree.Insert(&ACLNode{
		Path:          path,
		MandatoryACLs: p.MandatoryACLs,
		AllowedACLs:   p.OptionalACLs,
	})
	for _, s := range sharesWithPaths {
		tree.Insert(s.toTreeNode())
	}

	// Now that we have the ideal state, we need to get the
	// actual state of the namespace into something parsable.
	// We start by taking a dump of the namespace, and then we
	// parse this entry-by-entry
	dump, err := nsdumper.Dump(path, 0)
	if err != nil {
		return nil, nil, err
	}

	// Now we do the diff and get the resulting ChangeSet
	changeSet := compare(tree, dump)

	return changeSet, seen, nil
}

func (j *DeepJob) applyChangeSet(ctx context.Context, client eosclient.EOSClient, cs ChangeSet) error {
	// We are a sudo'er, so we don't set a role
	auth := eosclient.Authorization{}

	for _, c := range cs {
		var err error
		switch c.Action {
		case ActionAdd, ActionUpdate:
			err = client.AddACL(ctx, auth, c.Path, eosclient.StartPosition, c.ACL, false)
		case ActionDelete:
			// RemoveACL clears the permissions of the entry it is given, so we pass a copy
			e := *c.ACL
			err = client.RemoveACL(ctx, auth, c.Path, &e, false)
		default:
			return errors.Errorf("unknown action %s", c.Action)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to %s %s on %s", c.Action, c.ACL.CitrineSerialize(), c.Path)
		}
	}

	return nil
}

func (j *DeepJob) writeJournal(log *zerolog.Logger, report *DeepReport, cs ChangeSet, applyErr error) {
	for _, c := range cs {
		switch c.Action {
		case ActionAdd:
			report.Added++
		case ActionUpdate:
			report.Updated++
		case ActionDelete:
			report.Deleted++
		}
		log.Info().
			Str("event", EventDeepChange).
			Str("action", c.Action.String()).
			Str("path", c.Path).
			Str("type", c.ACL.Type).
			Str("qualifier", c.ACL.Qualifier).
			Str("permissions", c.ACL.Permissions).
			Bool("dry_run", j.DryRun).
			Msg("reconciliation: acl change")
	}

	if applyErr != nil {
		report.Failed++
		// We stop at the first failure, so the changes that come after the one
		// named here are not on the storage
		log.Error().Err(applyErr).
			Str("event", EventDeepFail).
			Msg("reconciliation: applying the change set failed")
	}
}

// progress reports the run, with the totals so far.
func (r *DeepReport) progress(rep rjobs.Reporter, done, total int) {
	rep.Report(rjobs.Progress{Phase: "reconcile", Done: int64(done), Total: int64(total), Unit: "spaces", Details: rjobs.Params{
		"added":          r.Added,
		"updated":        r.Updated,
		"deleted":        r.Deleted,
		"skipped_spaces": len(r.SkippedSpaces),
		"failed":         r.Failed,
	}})
}

// gateway returns the injected client when set, otherwise the one the service
// registry resolves right now.
func (j *DeepJob) gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	if j.Gateway != nil {
		return j.Gateway, nil
	}
	return service.Gateway(ctx)
}

func getPath(ctx context.Context, gw gateway.GatewayAPIClient, rid *provider.ResourceId) (string, bool) {
	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: rid},
	})
	if err != nil {
		return "", false
	}
	if statRes.Status == nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		return "", false
	}
	return statRes.Info.Path, true
}

func roleToAcls(role *provider.SpaceRole) []*acl.Entry {
	entries := []*acl.Entry{}
	for _, r := range role.Recipients {
		e := &acl.Entry{}
		e.Permissions = permissionsToACLPerms(role.PermissionSet)
		switch r.Type {
		case provider.GranteeType_GRANTEE_TYPE_GROUP:
			e.Type = acl.TypeGroup
			e.Qualifier = r.GetGroupId().OpaqueId
		case provider.GranteeType_GRANTEE_TYPE_USER:
			e.Type = acl.TypeUser
			e.Qualifier = r.GetUserId().OpaqueId
		}
		// a project without a group for a role still has the role, with an
		// empty recipient. That is not an entry EOS can hold.
		if e.Qualifier == "" {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

func compare(tree *ACLTree, ns *nsdump.NamespaceDump) ChangeSet {
	changeSet := ChangeSet{}
	for _, e := range ns.Entries {
		changeSet = append(changeSet, compareEntry(tree, e)...)
	}
	return changeSet
}

func compareEntry(tree *ACLTree, entry nsdump.NameSpaceEntry) ChangeSet {
	m, o, ok := tree.Find(entry.Path)
	if !ok {
		return nil
	}

	actualACLs := parseACLs(entry.XattrSysAcl)
	return calculateChangeSet(m, o, actualACLs, entry.Path)

}

func parseACLs(sysattr string) []*acl.Entry {
	acls, err := acl.Parse(sysattr, acl.ShortTextForm)
	if err != nil {
		return nil
	}
	return acls.Entries
}

func calculateChangeSet(mandatory, optional, actual []*acl.Entry, p string) ChangeSet {
	changeSet := ChangeSet{}

	// First calculate missing entries on `actual`
	for _, e := range mandatory {
		if !aclSetContains(actual, e) {
			changeSet = append(changeSet, &Change{
				Path:   p,
				Action: ActionAdd,
				ACL:    e,
			})
		}
	}

	// Then calculate which entries should not be there
	for _, e := range actual {
		if !aclSetContains(mandatory, e) && !aclSetContains(optional, e) {
			changeSet = append(changeSet, &Change{
				Path:   p,
				Action: ActionDelete,
				ACL:    e,
			})
		}
	}

	return changeSet
}

// We cannot use slices.Contains, because we want to compare
// the actual values, not the pointers
func aclSetContains(set []*acl.Entry, entry *acl.Entry) bool {
	return slices.ContainsFunc(set, func(c *acl.Entry) bool {
		return *c == *entry
	})
}

func shareToACL(s *collaborationv1beta1.Share) *acl.Entry {
	var e acl.Entry
	switch {
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP:
		e.Type = "egroup"
		e.Qualifier = s.Grantee.GetGroupId().GetOpaqueId()
	case s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER:
		e.Type = "u"
		e.Qualifier = s.Grantee.GetUserId().GetOpaqueId()
	default:
		return &e
	}

	e.Permissions = permissionsToACLPerms(s.Permissions.Permissions)
	return &e
}

func permissionsToACLPerms(p *provider.ResourcePermissions) string {
	// TODO(jgeens): we should define named constants for these int values
	// TODO(jgeens): we should define named constants for the EOS perms
	// TODO(jgeens): we should have some better error handling here
	ocs := permissions.OCSFromCS3Permission(p)
	switch ocs {
	case 0:
		return "!r!w!x"
	case 1:
		return "rx"
	case 15:
		return "rwx"
	}
	return ""
}

// DeepParams are the parameters of one on-demand run.
type DeepParams struct {
	// SpaceID limits the run to one space. Empty covers every space that holds
	// a share.
	SpaceID string `mapstructure:"space"`
}

// OnDemand has the shape of rjobs.NewJob, so it is what gets registered with
// rjobs.RegisterOnDemand. The job is already built, so the configuration
// section the runner passes is not read.
func (j *DeepJob) OnDemand(context.Context, map[string]any) (rjobs.Job, error) {
	return &deepRun{job: j}, nil
}

type deepRun struct {
	job *DeepJob
}

// Run performs one run and returns its totals as the run's result.
func (r *deepRun) Run(ctx context.Context, p rjobs.Params) (rjobs.Params, error) {
	return r.RunWithProgress(ctx, p, rjobs.NopReporter{})
}

// RunWithProgress is Run, reporting how far the run has got.
func (r *deepRun) RunWithProgress(ctx context.Context, p rjobs.Params, rep rjobs.Reporter) (rjobs.Params, error) {
	var params DeepParams
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
		return nil, errors.Wrap(err, "reconciliation: decoding the deep parameters")
	}

	report, err := r.job.run(ctx, params.SpaceID, rep)
	if err != nil {
		return nil, err
	}
	return rjobs.Params{
		"run":     report.RunID,
		"space":   report.SpaceID,
		"dry_run": report.DryRun,
		"spaces":  report.Spaces,
		"added":   report.Added,
		"updated": report.Updated,
		"deleted": report.Deleted,
		// the spaces left for the next run, named so that a run can be retried on
		// exactly the ones it held back.
		"skipped_spaces": report.SkippedSpaces,
		"failed":         report.Failed,
	}, nil
}
