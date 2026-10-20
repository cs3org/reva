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

// Command reconciliation-revert undoes the ACL changes a shallow reconciliation
// run made on the storage, from the run's journal alone. It never touches the
// share database: a share the run removed stays removed.
//
// For every grant the run wrote it reads the entry the storage holds now. An
// entry that is still what the run wrote is put back to what the run found: an
// added entry is removed, an updated one gets its old level back. An entry that
// changed since is left alone, so a change made after the run is never undone.
//
// It is a dry run unless -apply is given, and it keeps its own journal, which
// carries the level found and the level written for every change, so a revert
// can be undone the same way.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/registry"
	_ "github.com/cs3org/reva/v3/pkg/registry/loader"
	"github.com/cs3org/reva/v3/pkg/rgrpc/mtls"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// JobName is what every line of the revert journal carries under "job".
const JobName = reconciliation.ShallowJobName + ".revert"

// The events the revert logs.
const (
	// EventStart opens a revert.
	EventStart = JobName + ".start"
	// EventRevert reports one entry put back, or, in dry-run, one that would
	// have been. It carries the level found before and the level written, so
	// the revert can be undone.
	EventRevert = JobName + ".revert"
	// EventSkip reports one entry left alone, with the reason.
	EventSkip = JobName + ".skip"
	// EventFail reports one entry that had to be put back but could not be.
	EventFail = JobName + ".fail"
	// EventEnd closes a revert with its totals.
	EventEnd = JobName + ".end"
)

// The reasons an entry is skipped.
const (
	// ReasonReverted means the entry already holds what the run found.
	ReasonReverted = "already-reverted"
	// ReasonDrifted means the entry holds neither what the run found nor what
	// it wrote: it changed after the run, and that change is kept.
	ReasonDrifted = "drifted"
)

// The operations a revert applies.
const (
	OpRemove = "remove"
	OpUpdate = "update"
	OpDeny   = "deny"
)

// Grant is one grant the shallow run wrote, as its journal records it.
type Grant struct {
	Event       string `json:"event"`
	Run         string `json:"run"`
	Share       string `json:"share"`
	Action      string `json:"action"`
	Path        string `json:"path"`
	StorageID   string `json:"storage_id"`
	OpaqueID    string `json:"opaque_id"`
	Grantee     string `json:"grantee"`
	GranteeType string `json:"grantee_type"`
	Observed    string `json:"observed"`
	Expected    string `json:"expected"`
	DryRun      bool   `json:"dry_run"`
}

func (g Grant) key() string {
	return g.StorageID + "\x00" + g.OpaqueID + "\x00" + g.GranteeType + "\x00" + g.Grantee
}

// ReadJournal returns the grants the run wrote, in the order it wrote them. A
// dry-run line wrote nothing and is left out.
//
// An entry can be written more than once in a run, by two shares on the same
// resource for the same recipient. Only the first write is kept: what it found
// is what the storage held before the run.
//
// run selects the run. When empty the journal has to hold exactly one run that
// wrote something.
func ReadJournal(r io.Reader, run string) ([]Grant, string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	runs := map[string][]Grant{}
	for n := 1; sc.Scan(); n++ {
		var g Grant
		if err := json.Unmarshal(sc.Bytes(), &g); err != nil {
			return nil, "", errors.Wrapf(err, "journal line %d", n)
		}
		if g.Event != reconciliation.EventShallowGrant || g.DryRun {
			continue
		}
		if run != "" && g.Run != run {
			continue
		}
		runs[g.Run] = append(runs[g.Run], g)
	}
	if err := sc.Err(); err != nil {
		return nil, "", errors.Wrap(err, "reading the journal")
	}

	switch {
	case run != "" && len(runs) == 0:
		return nil, "", errors.Errorf("the journal holds no grant written by run %s", run)
	case len(runs) == 0:
		return nil, "", errors.New("the journal holds no grant written outside dry-run")
	case len(runs) > 1:
		ids := make([]string, 0, len(runs))
		for id := range runs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, "", errors.Errorf("the journal holds more than one run, pick one with -run: %v", ids)
	}

	for id, grants := range runs {
		seen := map[string]struct{}{}
		out := make([]Grant, 0, len(grants))
		for _, g := range grants {
			if _, ok := seen[g.key()]; ok {
				continue
			}
			seen[g.key()] = struct{}{}
			out = append(out, g)
		}
		return out, id, nil
	}
	panic("unreachable")
}

// GrantAPI is the part of the storage provider API the revert needs.
type GrantAPI interface {
	ListGrants(ctx context.Context, in *provider.ListGrantsRequest, opts ...grpc.CallOption) (*provider.ListGrantsResponse, error)
	RemoveGrant(ctx context.Context, in *provider.RemoveGrantRequest, opts ...grpc.CallOption) (*provider.RemoveGrantResponse, error)
	UpdateGrant(ctx context.Context, in *provider.UpdateGrantRequest, opts ...grpc.CallOption) (*provider.UpdateGrantResponse, error)
	DenyGrant(ctx context.Context, in *provider.DenyGrantRequest, opts ...grpc.CallOption) (*provider.DenyGrantResponse, error)
}

// Reverter puts back the entries a run wrote.
type Reverter struct {
	// Grants returns the grant API of the provider hosting storageID.
	Grants func(ctx context.Context, storageID string) (GrantAPI, error)
	// Grantee resolves a recipient from its journal form. A user has to be
	// resolved, because its type decides where the storage keeps its entry.
	Grantee func(ctx context.Context, granteeType, name string) (*provider.Grantee, error)
	Log     *zerolog.Logger
	// Out gets one line per entry, for the operator. Nil prints nothing.
	Out io.Writer
	// Apply writes the changes. Without it the revert only reports them.
	Apply bool
}

// Report sums up a revert.
type Report struct {
	Reverted int
	Skipped  int
	Failed   int
}

// Revert puts back every grant, in the order the run wrote them. The run wrote
// parents before children, and an entry on a folder is written to the whole
// tree below it, so a child is put back after its parent, never overwritten by
// it.
func (r *Reverter) Revert(ctx context.Context, grants []Grant) (Report, error) {
	var rep Report
	for i, g := range grants {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		r.printf("[%d/%d] share %s, %s %s:%s on %s\n", i+1, len(grants), g.Share, g.Action, g.GranteeType, g.Grantee, g.Path)
		switch r.revert(ctx, g) {
		case outcomeReverted:
			rep.Reverted++
		case outcomeSkipped:
			rep.Skipped++
		default:
			rep.Failed++
		}
	}
	return rep, nil
}

type outcome int

const (
	outcomeReverted outcome = iota
	outcomeSkipped
	outcomeFailed
)

func (r *Reverter) revert(ctx context.Context, g Grant) outcome {
	traceID := trace.Generate()
	ctx = trace.Set(ctx, traceID)
	fail := func(err error, before string) outcome {
		r.printf("    FAIL: %v (traceid %s)\n", err, traceID)
		r.entry(r.Log.Error().Err(err), g, traceID).
			Str("event", EventFail).
			Str("before", before).
			Msg("revert: putting the entry back failed")
		return outcomeFailed
	}

	expected, err := parseLevel(g.Expected)
	if err != nil {
		return fail(err, "")
	}
	var observed sharehierarchy.PermLevel
	switch g.Action {
	case "add":
	case "update":
		if observed, err = parseLevel(g.Observed); err != nil {
			return fail(err, "")
		}
	default:
		return fail(errors.Errorf("unknown action %q", g.Action), "")
	}

	grantee, err := r.Grantee(ctx, g.GranteeType, g.Grantee)
	if err != nil {
		return fail(err, "")
	}
	store, err := r.Grants(ctx, g.StorageID)
	if err != nil {
		return fail(err, "")
	}
	ref := &provider.Reference{ResourceId: &provider.ResourceId{StorageId: g.StorageID, OpaqueId: g.OpaqueID}}

	level, present, err := current(ctx, store, ref, g)
	if err != nil {
		return fail(err, "")
	}
	before := levelString(level, present)

	skip := func(reason string) outcome {
		r.printf("    skip: %s (now %q, run found %q, run wrote %q)\n", reason, before, g.Observed, g.Expected)
		r.entry(r.Log.Info(), g, traceID).
			Str("event", EventSkip).
			Str("reason", reason).
			Str("before", before).
			Msg("revert: entry left alone")
		return outcomeSkipped
	}

	// what the run found is what goes back, and only over what the run wrote.
	var op string
	switch {
	case g.Action == "add" && !present:
		return skip(ReasonReverted)
	case g.Action == "update" && present && level == observed:
		return skip(ReasonReverted)
	case !present || level != expected:
		return skip(ReasonDrifted)
	case g.Action == "add":
		op = OpRemove
	case observed == sharehierarchy.PermDeny:
		op = OpDeny
	default:
		op = OpUpdate
	}

	if r.Apply {
		if err := write(ctx, store, ref, grantee, op, expected, observed); err != nil {
			return fail(err, before)
		}
		// read back, so the journal holds what the storage has and not only
		// what was asked of it.
		after, ok, err := current(ctx, store, ref, g)
		if err != nil {
			return fail(errors.Wrap(err, "reading the entry back"), before)
		}
		want := g.Observed
		if got := levelString(after, ok); got != want {
			return fail(errors.Errorf("entry reads %q after the revert, want %q", got, want), before)
		}
	}

	verb := "reverted"
	if !r.Apply {
		verb = "would revert"
	}
	r.printf("    %s: %s, %q -> %q\n", verb, op, before, g.Observed)
	r.entry(r.Log.Info(), g, traceID).
		Str("event", EventRevert).
		Str("op", op).
		Str("before", before).
		Str("written", g.Observed).
		Bool("dry_run", !r.Apply).
		Msg("revert: entry put back")
	return outcomeReverted
}

func (r *Reverter) printf(format string, args ...any) {
	if r.Out != nil {
		fmt.Fprintf(r.Out, format, args...)
	}
}

// entry adds the fields that identify an entry and the run line it reverts.
func (r *Reverter) entry(e *zerolog.Event, g Grant, traceID string) *zerolog.Event {
	return e.
		Str("share", g.Share).
		Str("source_action", g.Action).
		Str("path", g.Path).
		Str("storage_id", g.StorageID).
		Str("opaque_id", g.OpaqueID).
		Str("grantee", g.Grantee).
		Str("grantee_type", g.GranteeType).
		Str("source_observed", g.Observed).
		Str("source_expected", g.Expected).
		Str("traceid", traceID)
}

// current reads the level the storage grants the recipient now, the same way
// the shallow job read it. The second result is false when there is no entry.
func current(ctx context.Context, store GrantAPI, ref *provider.Reference, g Grant) (sharehierarchy.PermLevel, bool, error) {
	res, err := store.ListGrants(ctx, &provider.ListGrantsRequest{Ref: ref})
	if err != nil {
		return 0, false, errors.Wrap(err, "list grants")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return 0, false, errors.Errorf("list grants: %s: %s", code, res.GetStatus().GetMessage())
	}
	for _, gr := range res.GetGrants() {
		if sharee, t := sharehierarchy.ShareeInfo(gr.GetGrantee()); sharee == g.Grantee && t == g.GranteeType {
			return sharehierarchy.PermLevelFromCS3(gr.GetPermissions()), true, nil
		}
	}
	return 0, false, nil
}

// write applies one revert operation.
func write(ctx context.Context, store GrantAPI, ref *provider.Reference, grantee *provider.Grantee, op string, expected, observed sharehierarchy.PermLevel) error {
	var st *rpc.Status
	switch op {
	case OpRemove:
		// the storage needs the permissions to build the entry it drops.
		res, err := store.RemoveGrant(ctx, &provider.RemoveGrantRequest{
			Ref:   ref,
			Grant: &provider.Grant{Grantee: grantee, Permissions: levelPermissions(expected)},
		})
		if err != nil {
			return errors.Wrap(err, "remove grant")
		}
		st = res.GetStatus()
	case OpUpdate:
		res, err := store.UpdateGrant(ctx, &provider.UpdateGrantRequest{
			Ref:   ref,
			Grant: &provider.Grant{Grantee: grantee, Permissions: levelPermissions(observed)},
		})
		if err != nil {
			return errors.Wrap(err, "update grant")
		}
		st = res.GetStatus()
	case OpDeny:
		res, err := store.DenyGrant(ctx, &provider.DenyGrantRequest{Ref: ref, Grantee: grantee})
		if err != nil {
			return errors.Wrap(err, "deny grant")
		}
		st = res.GetStatus()
	default:
		return errors.Errorf("unknown operation %q", op)
	}
	if code := st.GetCode(); code != rpc.Code_CODE_OK {
		return errors.Errorf("%s grant: %s: %s", op, code, st.GetMessage())
	}
	return nil
}

func parseLevel(s string) (sharehierarchy.PermLevel, error) {
	for _, l := range []sharehierarchy.PermLevel{sharehierarchy.PermRead, sharehierarchy.PermRW, sharehierarchy.PermDeny} {
		if l.String() == s {
			return l, nil
		}
	}
	return 0, errors.Errorf("unknown permission level %q", s)
}

// levelString is the journal form of an entry: its level, or empty for none.
func levelString(l sharehierarchy.PermLevel, present bool) string {
	if !present {
		return ""
	}
	return l.String()
}

// levelPermissions are the permissions a share of that level writes, the
// permissions of its role.
func levelPermissions(l sharehierarchy.PermLevel) *provider.ResourcePermissions {
	switch l {
	case sharehierarchy.PermRead:
		return permissions.NewViewerRole().CS3ResourcePermissions()
	case sharehierarchy.PermRW:
		return permissions.NewEditorRole().CS3ResourcePermissions()
	default:
		return &provider.ResourcePermissions{}
	}
}

// serviceConfig is the part of the reconciliation service configuration the
// revert reads: it acts as the same account as the job did.
type serviceConfig struct {
	JWTSecret       string `mapstructure:"jwt_secret"`
	ServiceUserName string `mapstructure:"service_user_name"`
	ServiceUserUID  int64  `mapstructure:"service_user_uid"`
	ServiceUserGID  int64  `mapstructure:"service_user_gid"`
}

func main() {
	configFile := flag.String("c", "/etc/revad/revad-reconciliation.toml", "revad configuration holding the reconciliation service")
	// named revad-* so the logrotate rule for /var/log/revad/revad*.log picks it up.
	logFile := flag.String("log", "/var/log/revad/revad-reconciliation-shallow-revert.log", `revert journal, or "stdout" or "stderr"`)
	run := flag.String("run", "", "run to revert, needed only when the journal holds more than one")
	apply := flag.Bool("apply", false, "write the changes; without it nothing is written")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] <shallow run journal>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := revert(flag.Arg(0), *configFile, *logFile, *run, *apply); err != nil {
		fmt.Fprintf(os.Stderr, "reconciliation-revert: %v\n", err)
		os.Exit(1)
	}
}

func revert(journal, configFile, logFile, run string, apply bool) error {
	f, err := os.Open(journal)
	if err != nil {
		return err
	}
	grants, runID, err := ReadJournal(f, run)
	f.Close()
	if err != nil {
		return err
	}
	fmt.Printf("journal %s: run %s, %d entries to check\n", journal, runID, len(grants))

	cfg, sc, err := loadConfig(configFile)
	if err != nil {
		return err
	}
	fmt.Printf("config %s: acting as %s (uid %d, gid %d), registry driver %q\n",
		configFile, sc.ServiceUserName, sc.ServiceUserUID, sc.ServiceUserGID, cfg.Shared.Registry.Driver)
	ctx, closeRegistry, err := connect(context.Background(), cfg, sc)
	if err != nil {
		return err
	}
	defer closeRegistry()

	base, logClose, err := reconciliation.OpenLog(logFile)
	if err != nil {
		return err
	}
	if logClose != nil {
		defer logClose.Close()
	}
	log := base.With().Str("job", JobName).Str("run", uuid.New().String()).Str("source_run", runID).Logger()

	gw, err := service.Gateway(ctx)
	if err != nil {
		return errors.Wrap(err, "getting the gateway client")
	}
	r := &Reverter{
		Grants:  (&storageProviders{clients: map[string]GrantAPI{}}).grants,
		Grantee: (&grantees{gw: gw, cache: map[string]*provider.Grantee{}}).resolve,
		Log:     &log,
		Out:     os.Stdout,
		Apply:   apply,
	}
	if apply {
		fmt.Printf("APPLY: writing changes to EOS, journal %s\n", logFile)
	} else {
		fmt.Printf("dry run: nothing is written, journal %s\n", logFile)
	}

	log.Info().
		Str("event", EventStart).
		Str("journal", journal).
		Int("entries", len(grants)).
		Bool("dry_run", !apply).
		Msg("revert: started")
	rep, err := r.Revert(ctx, grants)
	log.Info().
		Str("event", EventEnd).
		Int("reverted", rep.Reverted).
		Int("skipped", rep.Skipped).
		Int("failed", rep.Failed).
		Bool("dry_run", !apply).
		Msg("revert: finished")

	fmt.Printf("run %s: %d entries, %d reverted, %d skipped, %d failed (dry run: %t)\n",
		runID, len(grants), rep.Reverted, rep.Skipped, rep.Failed, !apply)
	if err != nil {
		return err
	}
	if rep.Failed > 0 {
		return errors.Errorf("%d entries could not be put back, see %s", rep.Failed, logFile)
	}
	return nil
}

// loadConfig reads the revad configuration and the reconciliation service in
// it.
func loadConfig(file string) (*config.Config, *serviceConfig, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	cfg, err := config.Load(f)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "loading %s", file)
	}
	if err := cfg.ApplyTemplates(cfg); err != nil {
		return nil, nil, errors.Wrapf(err, "applying the templates of %s", file)
	}

	raw, ok := cfg.Serverless.Services["reconciliation"]
	if !ok {
		return nil, nil, errors.Errorf("%s has no [serverless.services.reconciliation]", file)
	}
	var sc serviceConfig
	if err := mapstructure.Decode(raw, &sc); err != nil {
		return nil, nil, errors.Wrap(err, "decoding the reconciliation service")
	}
	if sc.JWTSecret == "" {
		sc.JWTSecret = cfg.Shared.JWTSecret
	}
	if sc.ServiceUserName == "" || sc.ServiceUserUID == 0 || sc.ServiceUserGID == 0 {
		return nil, nil, errors.New("the reconciliation service has no complete service user")
	}
	return cfg, &sc, nil
}

// connect sets up the service registry the way revad does and returns a
// context carrying a token for the service user.
func connect(ctx context.Context, cfg *config.Config, sc *serviceConfig) (context.Context, func(), error) {
	sharedconf.Init(cfg.Shared)

	rc := cfg.Shared.Registry
	reg, err := registry.New(rc.Driver, rc.Drivers[rc.Driver], registry.Thresholds{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "building the service registry")
	}
	closeRegistry := func() {
		if c, ok := reg.(interface{ Close() }); ok {
			c.Close()
		}
	}
	selector, err := service.NewSelector(rc.Selector)
	if err != nil {
		closeRegistry()
		return nil, nil, err
	}
	creds, err := mtls.ClientCredentials(cfg.Shared.MTLS)
	if err != nil {
		closeRegistry()
		return nil, nil, err
	}
	service.SetDialCredentials(creds)
	service.SetGlobal(service.NewClients(reg, selector))

	tokens, err := jwt.New(map[string]any{"secret": sc.JWTSecret})
	if err != nil {
		closeRegistry()
		return nil, nil, errors.Wrap(err, "building the token manager")
	}
	scopes, err := scope.AddOwnerScope(map[string]*authpb.Scope{})
	if err != nil {
		closeRegistry()
		return nil, nil, err
	}
	u := &userpb.User{
		Id:        &userpb.UserId{OpaqueId: sc.ServiceUserName, Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username:  sc.ServiceUserName,
		UidNumber: sc.ServiceUserUID,
		GidNumber: sc.ServiceUserGID,
	}
	tkn, err := tokens.MintToken(ctx, u, scopes)
	if err != nil {
		closeRegistry()
		return nil, nil, errors.Wrap(err, "minting the service token")
	}
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn), closeRegistry, nil
}

// storageProviders hands out the grant API of the provider hosting a storage,
// as the reconciliation service does for the job.
type storageProviders struct {
	mu      sync.Mutex
	clients map[string]GrantAPI
}

func (p *storageProviders) grants(ctx context.Context, storageID string) (GrantAPI, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[storageID]; ok {
		return c, nil
	}

	// a registry that is not a process-local one fills its cache from the
	// shared store after it starts, so the first lookups may come back empty.
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		addr, err := providerAddress(ctx, storageID)
		if err != nil {
			lastErr = err
			continue
		}
		c, err := service.StorageProviderAt(addr)
		if err != nil {
			return nil, errors.Wrap(err, "getting the storage provider client")
		}
		p.clients[storageID] = c
		return c, nil
	}
	return nil, lastErr
}

func providerAddress(ctx context.Context, storageID string) (string, error) {
	reg, err := service.StorageRegistry(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting the storage registry client")
	}
	res, err := reg.GetStorageProviders(ctx, &registrypb.GetStorageProvidersRequest{
		Ref: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: storageID}},
	})
	if err != nil {
		return "", errors.Wrap(err, "looking up the storage provider")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return "", errors.Errorf("looking up the storage provider: %s: %s", code, res.GetStatus().GetMessage())
	}
	if len(res.GetProviders()) == 0 {
		return "", errors.Errorf("no storage provider for %q", storageID)
	}
	return res.GetProviders()[0].GetAddress(), nil
}

// grantees resolves recipients the way the job did: a user through the
// gateway, so its type is the real one and a lightweight account is handled as
// one.
type grantees struct {
	gw    gateway.GatewayAPIClient
	cache map[string]*provider.Grantee
}

func (g *grantees) resolve(ctx context.Context, granteeType, name string) (*provider.Grantee, error) {
	switch granteeType {
	case sharehierarchy.ShareeTypeGroup:
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: name}},
		}, nil
	case sharehierarchy.ShareeTypeUser:
	default:
		return nil, errors.Errorf("unknown grantee type %q", granteeType)
	}

	if v, ok := g.cache[name]; ok {
		return v, nil
	}
	res, err := g.gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username", Value: name, SkipFetchingUserGroups: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "get user")
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return nil, errors.Errorf("get user %q: %s: %s", name, code, res.GetStatus().GetMessage())
	}
	v := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: res.GetUser().GetId()},
	}
	g.cache[name] = v
	return v, nil
}
