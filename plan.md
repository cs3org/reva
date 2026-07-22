# Share reconciliation implementation plan

## Context

We run a service called CERNBox, similar to Google Drive. It uses a backend server called Reva. Reva consists of multiple microservices, which are interconnected via standardized API's called the CS3APIs. To do operator-only operations, we have a dedicated tool, called cernboxcop.

The problem we are trying to solve is the following. Users can share resources with other users. This results in an ACL entry on the storage, as well as an entry in the database. For multiple reasons, these can diverge. The goal is to implement algorithms that fix this divergence. 

Some background info: storage in CERNBox is split up across "spaces". Spaces are completely disjunct. There are two types of spaces to take into account: personal ones (one per user account), and project spaces (for collaborative work between users).

Shares can also go to different recipients. Specifically, there are
1) CERN user acounts. These go directly in the actual ACLs. 
2) Groups. Also go in the ACLs. In the share definition, recipient type (user or group) is determined by share_with_is_group
3) External accounts. Do not go in the native EOS acls, but have a dedicated attribute: `sys.reva.lwshare.<email>=<acl>`

Users can be resolved via the reva gateway. ACLs (native ones, but also lightweight) should all be set via the CS3API. Note that even lightweight ACLs should be set via AddACL.

We use a three-level approach. 
1. The first is to detect shares in the database that are no longer valid. This can be because the file has been deleted, the recipient no longer exists, etc. We mark these invalid shares as "orphan".

2. The second is to list all spaces. Then for every space, we reconstruct what the ACLs *should* be on all the paths that are shared. Then, we check these paths and set the correctly if there is any difference.

3. Check the whole namespace. We once again list all the spaces. But then we use `eos-ns-inspect` and compare the whole namespace against what the database tells us

Note that spaces will have default ACLs. There are global default ACLs that are *allowed* to be anywhere but don't have to (`cbackeosro` and `cboxexternal`). Then for user spaces the owner of the space HAS to be everywhere. And for project spaces, there are three groups (one for readers, writers and admins each) that also have to be everywhere.

We should have an extremely extensive test suite. This test suite should cover the three levels. Every time, we need tests for:
* every recipient type
* all combo's of ACLs (see share hierarchy for possibilities)
* real eos-ns-inspect output parsing

Everything should also be very configurable. For example, we want to be able to set path_prefixes and map these to default ACL's, and we want to be able to say if they *can* be there or *should* be there.

Implementation details:
* We want three levels to run as three different jobs
* Please think about where the best place would be to put the code for the jobs. Perhaps together with the share hierarchy? Or under EOS (although this should also work for other storage drivers)?
* Use Reva's built-in jobs framework for running the jobs
* We already have a half-baked implementation under ~/Code/cernboxcop. This might be useful to inspect for the eos-ns-inspect code.
* Implement a dry_run mode so we can do dry runs on production data without modifying anything

## Implementation strategy

### Where the code lives

Put the reconciliation engine in a new top-level package `pkg/reconciliation`, not under
`pkg/storage/fs/eos` and not folded into `pkg/sharehierarchy`.

Reasoning:
* The three jobs are cross-cutting: they read the share DB (`pkg/share/manager/sql`),
  resolve identities through the gateway, list spaces, and mutate ACLs. None of those
  belong to a single storage driver, so `pkg/storage/fs/eos` is the wrong home. The
  requirement that this "should also work for other storage drivers" makes the driver
  package a dead end.
* `pkg/sharehierarchy` already owns the permission-ordering algebra (`PermLevel`,
  `PermLevelFromCS3`, ancestor/descendant resolution). Reconciliation *reuses* that, but
  it is a much bigger surface (namespace scanning, orphan detection, ACL diffing, jobs).
  Keep `sharehierarchy` as the pure algorithm library and let `pkg/reconciliation` depend
  on it. Do not grow `sharehierarchy` into a jobs package.

The one genuinely EOS-specific piece is reading the EOS namespace for level 3, which is what
`eos-ns-inspect` does. That does **not** live under `pkg/reconciliation`. It belongs with the
rest of the EOS driver under `pkg/storage/fs/eos`, exactly like the existing eos client,
grant, and recycle code. `pkg/reconciliation` only defines the `NamespaceScanner` interface
and a small registry; the EOS driver provides the concrete scanner and registers it from its
own `init()`, the same `Register(name, NewFunc)` + `loader.go` pattern reva already uses for
storage backends (`pkg/storage/fs/registry`). This keeps EOS code with EOS and keeps the
engine free of any EOS import.

What "read the EOS namespace" means, and why it is not the MGM gRPC API (established while
writing this plan, from the EOS source under `~/Code/eos`):
* `eos-ns-inspect` reads **QDB directly** (QuarkDB: RocksDB behind a Redis-protocol server),
  not the MGM. `Find` / the reva `EOSClient.ListWithRegex` gRPC call goes through the running
  MGM and is a different operation; it is not a substitute for a whole-namespace audit and
  would load the live MGM. So level 3 uses a QDB-reading scanner, not gRPC.
* In QDB the namespace is protobuf blobs: all container MDs under one "locality hash" key,
  all file MDs under another, read with QDB-custom commands (`LHGET`/`LHSCAN`), with
  parent/child links in standard Redis hashes `<id>:map_conts` and `<id>:map_files`
  (`HSCAN`/`HGET`), root container id = 1. Each `FileMdProto` / `ContainerMdProto`
  (`proto/namespace/ns_quarkdb/{FileMd,ContainerMd}.proto`) carries the `xattrs` map, where
  `sys.acl` and the lightweight `sys.reva.lwshare.<email>` entries live, plus uid/gid/name.

Decision (see "Open questions"): define the `NamespaceScanner` interface now and, for now,
ship a single EOS implementation behind it that shells out to the binary. A native QDB reader
is a possible future option, not part of this work:
* `eos-nsinspect-binary` (what we build): exec the version-matched `eos-ns-inspect scan ... --json`
  binary and parse its output, as cernboxcop does. Least code, no coupling to the QDB on-disk
  schema, fastest path to a working level 3. Needs the binary and keytab on the host running
  the job. This is the only scanner we implement now.
* Native QDB reader (**could consider later**, not scheduled): a Go QDB reader that does what
  the binary does in-process: a redis-protocol client issuing `LHSCAN`/`HSCAN`/`LHGET`, the QDB
  **HMAC challenge-response handshake** for auth (QDB does not use plain redis `AUTH`; this is
  the fiddliest part, would be ported from `qclient`), generated Go types from the two `.proto`
  files, and the flat or tree scan. It would remove the external-binary dependency and be fully
  unit-testable with recorded QDB responses, at the cost of tracking the EOS QDB schema across
  releases. Worth revisiting only if the on-host binary dependency becomes a real operational
  problem.
The point of the `NamespaceScanner` interface is that if that native reader is ever built, it
drops in behind the same interface: level 3 and its tests do not change, only the registered
scanner name in config would.

Decisions taken (see "Open questions" answers):
* Levels 2 and 3 are separate jobs. They share only the "what should the ACL be" computation
  (the `expected_acls.go` + `planner.go` pair below), not their scan or scheduling.
* Orphan detection (level 1) resolves recipients and resources through the gateway (CS3),
  not by reading EOS or the DB directly. Driver-agnostic and consistent with the rest.

Proposed layout. Two files carry the naming that needs explaining up front:
* `expected_acls.go` is the pure function "given the shares and defaults for a space, what
  ACL entries *should* exist on each path". This is the piece shared between levels 2 and 3.
* `planner.go` diffs those expected ACLs against the *observed* ACLs and produces a `Plan`:
  the ordered list of add/remove/update actions. `applier.go` then executes a `Plan`.

```
pkg/reconciliation/
  reconcile.go          // shared types: Space, Recipient, ExpectedACL, Plan, Action, Outcome
  config.go             // Config + ApplyDefaults, path_prefix -> default-ACL rules (can/should)
  default_acls.go           // default-ACL computation per space type (owner, project egroups, globals)
  expected_acls.go      // pure: (shares for a space) + defaults -> expected ACL set per path
                        //   (shared by levels 2 and 3; wraps sharehierarchy)
  planner.go            // pure: expected ACLs vs observed ACLs -> Plan of add/remove/update
  applier.go            // executes a Plan via the CS3 grant API; honours dry_run
  identity.go           // recipient resolution + classification (user / group / lightweight)
  scanner.go            // NamespaceScanner interface + Register/registry (driver-agnostic)
  orphans.go            // level 1
  space_acls.go         // level 2
  namespace.go          // level 3 (depends on the interface, never on the EOS impl)
  jobs/
    orphans_job.go      // rjobs on-demand + periodic registration for level 1
    spaceacls_job.go    // level 2
    namespace_job.go    // level 3

pkg/storage/fs/eos/            // EOS-specific scanner lives with the EOS driver
  nsscan_loader.go             // registers the scanner in init() (Register pattern)
  nsscan_binary.go             // eos-nsinspect-binary: exec eos-ns-inspect + JSON parser
                               //   (ported from cernboxcop); the only scanner we build now
  testdata/nsinspect/          // captured real eos-ns-inspect JSON (binary scanner tests)
  // A native QDB reader (nsscan_qdb.go + a qdb/ client package) could be added here later
  // behind the same interface, but is out of scope for now.
```

The scanner sits behind the `NamespaceScanner` interface defined in `pkg/reconciliation`.
Level 3 depends only on that interface and looks the scanner up by name from the registry
(config: `scanner = "eos-nsinspect-binary"`). The concrete implementation lives under
`pkg/storage/fs/eos` and registers itself at init time, so `pkg/reconciliation` never imports
the EOS driver. A storage driver that cannot enumerate its namespace simply registers no
scanner, and level 3 is a no-op for its spaces; levels 1 and 2 stay driver-agnostic by going
through CS3.

### Reuse from cernboxcop

Port, do not import, from `~/Code/cernboxcop/pkg`:
* `eos/ns_inspect.go`: the `eos-ns-inspect scan ... --json` command builder and the
  `CommonEntry` / `DirEntry` / `FileEntry` parser. This is the `eos-nsinspect-binary` scanner
  and lands in `pkg/storage/fs/eos` (`nsscan_binary.go`), not in `pkg/reconciliation`. Keep
  the `prefetchedData` path: it is what makes real-output parsing testable and enables dry
  runs against a captured snapshot.
* `reconciliation/set_operations.go` and `acl_change_set.go`: the ACL set-diff
  (add / remove / update) logic is sound and becomes the core of `planner.go`.
* `reconciliation/permission_store.go` and `deep_fs.go`: the "reconstruct expected ACLs by
  walking parent shares" idea, reworked to use `sharehierarchy` for the permission ordering
  instead of the ad-hoc `rx`/`rwx` string comparison, and to be space-scoped from the start.

Fixes to make while porting:
* Replace `os/user.Lookup` and hardcoded `/eos/user/...` path math with gateway-based
  identity resolution and the space's own root path.
* Do not shell to EOS for mutations. `acl_change_set.go` currently calls the EOS client
  directly; route every mutation through the CS3 grant API (see below).
* Space isolation is mandatory. Every DB read filters by `space_id` (`SpaceIDFilter`), and
  hierarchy never crosses a space boundary. See the existing memory on share space isolation.

### Setting ACLs: always through CS3

All mutations go through the gateway grant API, never the EOS binary directly:
* Native user/group ACLs and lightweight (external) ACLs are all set with `AddGrant` /
  `RemoveGrant` / `UpdateGrant` / `DenyGrant` on the storage provider. The EOS driver
  (`pkg/storage/fs/eos/grant.go`) already routes lightweight accounts to the
  `sys.reva.lwshare.<email>` xattr and everything else to `sys.acl`, so the reconciler does
  not need to know the on-disk encoding. This is what keeps it driver-agnostic and is also
  the project rule (lightweight ACLs still go via AddGrant).
* Recipient classification drives the CS3 `Grantee`:
  * `share_with_is_group == true` -> `GranteeType_GROUP`.
  * external account (lightweight) -> user grantee whose id the driver recognises as
    lightweight; the driver picks the xattr path.
  * otherwise -> `GranteeType_USER`.
* Reconstruct `ResourcePermissions` from the DB `permissions` (OCS uint8) exactly as
  `model.Share.AsCS3Share` does, then map through `sharehierarchy.PermLevelFromCS3` when we
  need to compare levels. Permissions=0 is an active deny, not "no share" (see `PermDeny`).

### The three jobs

Each level is its own `rjobs` job, registered both on-demand (operator triggers a run,
optionally scoped to one space or user) and periodic (`ScopeLeader`, since they mutate
shared state and must fire once across replicas). Register under stable names, e.g.
`reconciliation.orphans`, `reconciliation.space_acls`, `reconciliation.namespace`. Config
per job comes from `[serverless.services.jobs.on_demand."reconciliation.namespace"]`.

**Level 1: orphan detection (`orphans.go`).**
List DB shares (`ListModelShares`, including orphans) per space. A share is an orphan when
its resource no longer resolves (gateway Stat returns not-found / in recycling), or its
recipient no longer exists (gateway user/group lookup), or the space is gone. All three
checks go through the gateway (CS3), never a direct EOS or DB read, so this stays
driver-agnostic. Mark with `ShareMgr.MarkAsOrphaned`. No ACL writes, so it is cheap and safe
to run frequently. Public links reuse the same pass via `PublicShareMgr`.

**Level 2: per-space expected-ACL reconstruction (`space_acls.go`).**
For each space: gather its non-orphan shares, group by grantee, and use `sharehierarchy` to
collapse each grantee's shares into the minimal correct ACL set per path (nearest-ancestor
wins, children with higher perms are re-applied). Add the space's default ACLs (below).
Stat each shared path, diff observed grants against expected with the planner, and apply.
This corrects the shared paths only; it does not walk the whole tree, so it is the routine
reconciler.

**Level 3: full-namespace sweep (`namespace.go`).**
List spaces, then for each look up the configured `NamespaceScanner` from the registry (the
EOS one shells out to the `eos-ns-inspect` binary, which reads QDB) and scan the whole space
subtree. For every node compute expected ACLs = default ACLs for the space + inherited
share ACLs from the permission store, diff against the scanned `sys.acl` (and lightweight
xattrs), and apply. This is the expensive, authoritative sweep; schedule it `@daily`/`@weekly`
with jitter and `Skip` overlap. It catches drift on paths that no share touches anymore.

Levels 2 and 3 are separate jobs but share `expected_acls.go` (what the ACLs should be) plus
`planner.go` and `applier.go` (diff and execute). They differ only in how they gather the
observed state (gateway Stat on shared paths vs. full eos-ns-inspect scan) and which node set
they cover. Level 1 shares none of this; it only reads and marks the DB.

### Default ACLs and configuration

`Config` (decoded from the job's config section) holds an ordered list of path-prefix rules:

```
[[path_prefix]]
  prefix = "/eos/user"
  [[path_prefix.default_acl]]
    type        = "u"                    # u | egroup | lw (see package acl)
    qualifier   = "{owner}"              # may contain {owner} / {project}
    permissions = "rwx"
    enforcement = "must"                 # "may" (allowed anywhere) | "must" (required everywhere)
  [[path_prefix.default_acl]]
    type        = "egroup"
    qualifier   = "cbackeosro"
    permissions = "rx"
    enforcement = "may"
```

A space is governed by the single rule whose prefix is a path prefix of its root. Prefixes may
not overlap (e.g. `/eos/user` vs `/eos/project`), so at most one rule matches and there is no
space_type or priority to reason about. The default ACL entry is given as explicit `type` /
`qualifier` / `permissions` rather than a single opaque token, so it is unambiguous and
validatable at config load. `default_acls.go` resolves the `{owner}` / `{project}` templates in
the qualifier per space.

Semantics, matching the spec:
* Global defaults (`cbackeosro`, `cboxexternal`): `enforcement = "may"`. Present is fine,
  absent is fine; never added, never removed by the reconciler.
* Personal space owner: `enforcement = "must"`, template resolves to the space owner uid.
  Missing => add; the planner never removes a "must" entry.
* Project spaces: the readers/writers/admins egroups are three `must` entries, templated
  from the project name.

`default_acls.go` resolves templates (`{owner}`, `{project}`) against the space. The planner
treats `must` entries as always-expected and `may` entries as never-diffed (neither added
nor flagged), so a `may` entry present on disk is left untouched.

### dry_run mode

`dry_run` is a `Config` bool threaded into `applier.go`. When set, the applier logs and
records each intended `Action` (path, grantee, before/after) into the job's result `Params`
and the run status, and skips the CS3 call entirely. Level 3 additionally accepts a
`prefetched_scan` path so a captured `eos-ns-inspect` snapshot can be replayed offline, so we
can dry-run against production data without touching EOS or the DB.

### Test suite

Tests live beside each file plus an integration layer in `pkg/reconciliation`. Coverage per
the spec, driven by table tests:
* Every recipient type: CERN user, group (`share_with_is_group`), external/lightweight.
  Assert each maps to the correct CS3 `Grantee` and, for lightweight, that the driver would
  target the `sys.reva.lwshare.<email>` xattr.
* Every ACL combination from the hierarchy: all ordered pairs of `{R, RW, Deny}` for
  parent/child on nested paths, plus the re-apply and delete cases already covered by
  `sharehierarchy` tests, now asserted end to end as `Plan` actions.
* Default-ACL rules: `may` present/absent (untouched), `must` present/absent (added), and
  wrong-perms `must` (updated), for personal and project spaces.
* Real `eos-ns-inspect` output: commit captured JSON under
  `pkg/storage/fs/eos/testdata/nsinspect` (personal and project space, files and folders,
  sys entries, lightweight xattrs). Assert the binary scanner's parser (EOS-driver test) and,
  feeding the scanner output into the engine, that the level-3 planner produces the expected
  `Plan` (reconciliation test). Reuse the `prefetchedData` path so no QDB or MGM is needed in
  CI. (If a native QDB scanner is ever added, it would get its own recorded-response tests and
  a cross-check asserting it yields the identical node set and ACLs as the binary scanner.)
* Orphan detection: deleted resource, recycled resource, missing recipient, missing space.
* dry_run: assert no mutation is issued and the recorded actions match what a live run would
  have applied (run planner once, apply in both modes, compare).


## Work breakdown

We build the simplest thing that works first, then deepen. The strategy above describes the
eventual full system; the phases below are the build order. Each phase is a self-contained,
reviewable unit that compiles and has its own tests, and each is useful on its own. A later
phase never blocks an earlier one.

Progress marker: `[x]` done, `[ ]` todo.

**Phase 1: orphan job.** `[x]` done.
`orphan.go`: a periodic job that scans the share DB and marks a share orphaned
when its resource or its recipient no longer exists. It lists non-orphan shares via
`ListModelShares(nil, nil, hideOrphans=true)`; for each it checks the resource
(`gateway.Stat` on `{Instance, Inode}`) and the recipient (`GetUserByClaim` for users,
`GetGroupByClaim` for groups), then marks via `MarkAsOrphaned`. A lookup error is never
treated as absence: the share is skipped, never orphaned on uncertainty. `dry_run` reports
what would be marked without mutating. Runs `ScopeLeader` because it mutates shared DB state.
Consumer-defined `ShareStore` and `ExistenceChecker` interfaces keep the logic unit-testable:
`*sql.ShareMgr` satisfies the first; the concrete CS3 gateway-backed `ExistenceChecker` is
built at service-startup wiring time (with the gateway address from config), not in this
package, so phase 1 carries no dead wiring.
Missing-space is folded into the resource check for now (if the space is gone the resource
Stat fails); a dedicated space check can come later.
Tests: resource missing, user recipient missing, group recipient missing, all present,
dry_run marks nothing, lookup error skips (no false orphan), already-orphan shares excluded,
mixed batch, share-reference by id.

**Phase 2: shallow check (DB only).** `[ ]`
Reconcile the ACLs implied by the share DB against what is actually set on each shared path,
without a full-namespace scan. For each non-orphan share, resolve its path and read the
current grants on that single node through the gateway, then add or fix the missing/wrong
grant. Reuses the default-ACL config (`config.go` / `default_acls.go`) and the permission ordering
from `sharehierarchy`. `dry_run`. Targeted and per-share, so cost scales with the number of
shares, not the size of the namespace.

**Phase 3: deep FS check (eos-ns-inspect).** `[ ]`
The whole-namespace sweep. Enumerate every node of a space directly from QuarkDB via
`eos-ns-inspect` (which reads the namespace, not the MGM), compare each node's actual
`sys.acl` against what the DB says it should be, and correct drift, including stray entries
that no share justifies. Behind a `NamespaceScanner` interface with the EOS binary scanner as
the first implementation; a native QDB reader could come later. `dry_run`.
