# AGENTS.md

Notes for coding agents. 

## General Instructions

### Git hygiene

- Do not commit unless explicitly asked, and do not continue a rebase or cherry-pick. Resolve conflicts in the worktree and stop.
- Commit subjects can use `area: message` (for example `eos: ...`).
- Do not use Co-Authored-By
- If you made significant contributions to the code, you can add a trailer to the commit message `Assisted-by: <harness>:<model>`. E.g. `Assisted-by: Claude:claude-opus-5-5`.


### Code style

- Do not write a comment that tells what the code does. Instead, focus on the why: write a comment only for a reason or a 
  constraint that is not clear, in one or two lines. Do not copy the comment density of verbose files.
- Do not reword existing comments unless the change makes them wrong.
- Never use em-dashes in code, comments, docs or commit messages.
- Do not re-invent the standard library. In general, try to re-use existing code as much as possible. 
- Try to use ASD-STE100 when writing comments
- For non-trivial changes, always make an implementation plan and present this to the maintainer before doing the actual implementation
- Less is more

### Make it easy for maintainers

Never:
- Write complete solutions without first stepping through the proposed solution
- Use jargon without explaining it
- Rush through explanations

Always:
- Ask clarifying questions to understand what they've tried, or why a solution is not acceptable
- Explain the "why" behind every piece of guidance
- Break everything into small, digestible steps
- Point to resources when they need deeper understanding


## Project-specific Instructions

### Project overview

- Reva (module `github.com/cs3org/reva/v3`) is the backend of CERNBox. It connects web, desktop and mobile clients to storage (mainly EOS, also CephFS and local).
- `cmd/revad` is the daemon. One TOML config selects the gRPC, HTTP and serverless services that one process runs. `cmd/reva` is the CLI client.
- gRPC services in `internal/grpc/services` implement the CS3 APIs (`github.com/cs3org/go-cs3apis`). The gateway is the entry point. It routes calls to the providers (storage, users, groups, shares, auth, apps, OCM).
- HTTP services in `internal/http/services` expose client protocols: WebDAV (`owncloud/ocdav`), OCS (`owncloud/ocs`), Graph (`owncloud/ocgraph`), data transfer (`dataprovider`, `datagateway`), OCM and app integration.
- Serverless services in `internal/serverless/services` run background work (jobs, notifications, share reconciliation).
- `pkg/` holds the drivers and libraries. Storage drivers are in `pkg/storage/fs`. Share and public-link managers are in `pkg/share` and `pkg/publicshare`.

### Deployment

Reva at CERN is deployed via puppet on dedicated machines, not on the local machine. For the deployment config, look for a local `it-puppet-hostgroup-box` folder. For CERN's Reva build, we also include our own plugins (`github.com/cernbox/reva-plugins`).

### Build and test

- `make revad` builds `cmd/revad/revad`. The integration tests run this binary, so build it before `make test-integration`.
- `make test-go` runs unit tests with `-race` and excludes `tests/integration`.
- `make test-integration` needs Redis and MySQL. Set `REDIS_ADDRESS`, `SQL_USERNAME`, `SQL_PASSWORD`, `SQL_ADDRESS` and `SQL_DBNAME` (see `.github/workflows/test-integration.yml`).
- `make lint` is only `go vet ./...`.
- `make cernbox-revad` builds with the external `cernbox/reva-plugins` through gaia. The `*-local` variants use `../reva-plugins`.
- The only `go:generate` is in `pkg/share/share.go` (mockery v1). Gateway mocks come from `github.com/cs3org/go-cs3apis/mocks`.
- `tmp/` is gitignored. Integration tests write their temp roots there.

### Docs site

- The site is Hugo only, with its own theme in `docs/layouts`. There is no Node and no theme submodule.
- `make gen-doc` runs `tools/generate-documentation`. It writes `docs/content/en/docs/components/` (one page per service, middleware and driver) and `docs/data/generated/`. Do not edit the generated files by hand.
- A component page shows the package doc comment, the package `README.md`, and the `mapstructure` fields of the config struct. Field comments are the descriptions. Defaults come from `ApplyDefaults`, or from a `docs:"<default>;<description>"` tag. To improve a page, change the package, then run `make gen-doc`.
- A package `README.md` can start with a YAML header with `subsystem_name` (page title) and `subsystem_category` (group in the navigation). A header in a driver family's registry README applies to the whole family. See `docs/README.md`.
- `make gen-doc-contributors` also fetches the contributor list from the GitHub API.
- `docs/data/architecture.yaml` holds the home page diagram and its request traces. If you change a request flow in the code, update the trace.
- To validate a docs change, run Hugo in `docs/` with the `HUGO_VERSION` from `netlify.toml`. A passing build is enough. Screenshots are not necessary.
- If one field breaks the build, fix only that field.

### Registration of drivers and services

- Each driver or service registers itself in `init()` (`registry.Register`, `rgrpc.Register`, `global.Register`, `rserverless.Register`).
- A new driver is compiled in only if you add a blank import to the matching `loader/loader.go`. A new extension point also needs its loader in `cmd/revad/runtime/loader.go`.
- The root `plugins.go` is for external plugins (gaia). Nothing in-tree uses it.

### Config

- Decode with `cfg.Decode` (`pkg/utils/cfg`). It runs mapstructure, then `ApplyDefaults()`, then `validate` tags. Unknown keys are not rejected.
- Some callers apply defaults again, so `ApplyDefaults` must be idempotent.
- TOML values can use templates such as `{{ grpc.services.gateway.address }}`. A `template:"-"` tag disables this for a field.
- The production revad TOML templates are in `~/Code/it-puppet-hostgroup-box` (`code/templates/ocis/`, `data/hostgroup/box/ocis/*.yaml`). Read them before you make assumptions about the runtime config of a production problem.

### Architecture traps

- `pkg/appctx` holds the user, token, scopes and logger. There is no `ctxpkg`.
- HTTP services talk only to the gateway (`service.Gateway(ctx)` in `pkg/service`).
- `utils.Skip` (unprotected paths) is a prefix match. An unprotected service root makes all its child paths unprotected. Thus an anonymous service cannot protect its sibling endpoints. Put authenticated endpoints in a protected service (for example OCS).
- Auth scopes are matched by key prefix (`pkg/auth/scope/scope.go`).
- Public-link access: mint tokens for the synthetic guest user in the token (`USER_TYPE_GUEST`, opaque id `publiclink:<shareID>`). Never resolve or impersonate the folder owner. The owner can be root (uid 0) or a deleted account. Keep the auth interceptor independent of the identity and of the storage driver.
- CS3 `FindUsers` filters are AND-combined. Two `Filter_TYPE_USERTYPE` filters match nobody. Filter multiple user types in the caller (see `ocgraph/users.go`).
- Scale: about 1M users with about 50 spaces each. Do not key a cache on the user or on (user, space). Key it on the space and do the per-user part at lookup time.

### Shares

- The share table holds millions of rows. Never call `ListShares` with no filter (or a weak filter) and filter in Go. Add a SQL query with the correct `WHERE` clause to the sql manager.
- Hierarchy conflicts exist only inside one storage space. The `SpaceIDFilter` in `listSharesForGranteeInSpace` is mandatory. If shares are missing, the cause is upstream (`space_id` not set at create time).

### EOS driver

- `use_grpc` selects the client: `client/binary` (runs the `eos` CLI and `xrdcopy`) or `client/grpc` (MGM gRPC plus FST HTTP). A change to the shared client interface must go into both clients.
- Only system ACLs are used (`sys.acl`). We do not use user ACLs.
- File xattr writes go to the version folder (`.sys.v#.<file>`) so they survive overwrites. Reads must check the version folder first and then fall back to the file. `GetAttrs` merges both, and the version folder has priority. `UnsetAttr` must agree with this, or a fallback read brings a deleted value back.
- Lightweight (external account) ACLs are stored as `sys.reva.lwshare.<email>` xattrs.

## Code style

- Do not add small helper functions or `NewFoo()` constructors. Use struct literals. Find an existing function first (stdlib `slices`, `maps`, `strings`, or this repo).
- Propose new caches, config fields or plumbing before you write them.

## Tests

- Unit tests mix `testing`/testify and ginkgo v1. Integration tests (`tests/integration/grpc`) use ginkgo with TOML fixtures.
- Test behaviour, not implementation!S Do not edit the expected values of existing tests to match new code. Add a new test for new behaviour.
- Do not add tests for trivial changes, for glue code over a library, or tests that only repeat the implementation.

## Changelog

- CI wants a file in `changelog/unreleased/`. Agents must not write one. The maintainers add it.

