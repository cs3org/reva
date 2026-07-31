# agents.md — reva

## Repository Overview

Reva is ownCloud's fork of the CS3 Organization Reva project, an interoperability platform for cloud storage sync and share services. It implements the CS3 APIs to connect storage backends, sync clients, and application providers. Within the ownCloud ecosystem, Reva serves as the core storage layer for ownCloud Infinite Scale (oCIS).

- **Classification:** oCIS
- **Activity Status:** Active
- **License:** Apache-2.0 (this is a fork of cs3org/reva)
- **Language:** Go

## Architecture & Key Paths

- `cmd/` — CLI entry points (`revad`, `reva`)
- `pkg/` — Shared library packages (utils, storage drivers, auth, etc.)
- `internal/` — Internal packages not intended for external use
- `examples/` — Example configuration files for running revad
- `tests/` — Unit, integration, and acceptance tests
- `docs/` — Documentation sources (Netlify-deployed)
- `tools/` — Build and development tools
- `changelog/` — Unreleased changelog entries (calens format)
- `Makefile` — Build orchestration
- `go.mod` / `go.sum` — Go module definitions

## Development Conventions

- Go module project with `make` as the build system
- Changelog entries go in `changelog/` directory (processed by calens)
- `CONTRIBUTING.md` present at repo root
- `CODEOWNERS` file governs review assignments
- Two main branches: `master` (1.x stable) and `edge` (2.x development with Spaces)

## Build & Test Commands

```bash
make build                # Build revad and reva binaries
make test                 # Run unit tests
make test-integration     # Run integration tests (requires Redis)
make lint                 # Run golangci-lint
make lint-fix             # Run golangci-lint with auto-fix
make tidy                 # Run go mod tidy
make imports              # Format imports with goimports
make build-ci             # Build for CI environment
make gen-doc              # Generate documentation
make clean                # Clean build artifacts
```

## Important Constraints

- **Fork repository:** This is a fork of cs3org/reva. Upstream license is Apache-2.0 and cannot be changed unilaterally.
- **License:** Apache-2.0 — already aligned with the OSPO target license.
- **Apache 2.0 migration:** The broader ownCloud organization is migrating repositories to Apache 2.0. This repo is already Apache-2.0 licensed, but other oCIS-related repos may have copyleft (AGPL-3.0) dependencies that must be considered when integrating.
- **CS3 API compatibility:** Changes must remain compatible with the CS3 API specification.
- **Two active branches:** `master` (v1.x) and `edge` (v2.x) — PRs should target the correct branch.


## OSPO Policy Constraints

### GitHub Actions
- **Only** use actions owned by `owncloud`, created by GitHub (`actions/*`), verified on the GitHub Marketplace, or verified by the ownCloud Maintainers.
- Pin all actions to their full commit SHA (not tags): `uses: actions/checkout@<SHA> # vX.Y.Z`
- Never introduce actions from unverified third parties.

### Dependency Management
- Dependabot is configured for automated dependency updates.
- Review and merge Dependabot PRs as part of regular maintenance.
- Do not introduce new dependencies without discussion in an issue first.

### Git Workflow
- **Rebase policy**: Always rebase; never create merge commits. Use `git pull --rebase` and `git rebase` before pushing.
- **Signed commits**: All commits **must** be PGP/GPG signed (`git commit -S -s`).
- **DCO sign-off**: Every commit needs a `Signed-off-by` line (`git commit -s`).
- **Conventional Commits & Squash Merge**: Use the [Conventional Commits](https://www.conventionalcommits.org/) format where the repository enforces it. Many repos use squash merge, where the PR title becomes the commit message on the default branch — apply Conventional Commits format to PR titles as well. A reusable GitHub Actions workflow enforces this.

## Context for AI Agents

- The project uses Go with standard `make` build tooling.
- The `Dockerfile.revad` and `Dockerfile.reva` exist for container builds.
- Tests are layered: unit (`make test`), integration (`make test-integration`, needs Redis), Litmus (WebDAV), and ownCloud acceptance tests.
- Configuration uses TOML files (see `examples/` directory).
- The `pkg/` directory contains the bulk of the business logic organized by domain.
