# Reva

<!-- OSPO-managed README | Generated: 2026-04-16 | v2 -->

[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE) [![ownCloud OSPO](https://img.shields.io/badge/OSPO-ownCloud-blue)](https://kiteworks.com/opensource) [![Docker Hub](https://img.shields.io/docker/pulls/owncloud)](https://hub.docker.com/r/owncloud/ocis)

Reva is an interoperability platform for cloud storage sync and share services. It connects storage backends, sync clients and application providers in a vendor-neutral way by implementing the CS3 APIs, enabling seamless federation and data exchange between different EFSS (Enterprise File Sync and Share) systems.

## Part of oCIS

Reva serves as the core storage interoperability layer for [ownCloud Infinite Scale (oCIS)](https://github.com/owncloud/ocis). This is ownCloud's fork of the upstream [CS3 Organization Reva project](https://github.com/cs3org/reva), customized to work with oCIS-specific storage drivers and authentication mechanisms. It handles all CS3-compliant file operations including WebDAV, data gateways and user management within the oCIS architecture.

oCIS is available on [Docker Hub](https://hub.docker.com/r/owncloud/ocis).

> **Note:** This repository is a fork of the upstream CS3 Organization Reva project.

## Getting Started

You need [Go](https://golang.org/doc/install) (version 1.16 or higher), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed.

```bash
git clone https://github.com/owncloud/reva
cd reva
make build
```

To run:

```bash
mkdir -p /etc/revad
cp examples/storage-references/users.demo.json /etc/revad/users.json
cp examples/storage-references/groups.demo.json /etc/revad/groups.json
cp examples/storage-references/providers.demo.json /etc/revad/ocm-providers.json
cmd/revad/revad -dev-dir examples/storage-references
```

Run tests with:

```bash
make test                  # unit tests
make test-integration      # integration tests (requires Redis)
```

## Documentation

- [Reva Documentation](https://reva.link)
- [Getting Started Guide](https://reva.link/docs/getting-started/)
- [Build from Sources](https://reva.link/docs/getting-started/build-reva/)
- [Tutorials](https://reva.link/docs/tutorials/)
- [ownCloud oCIS Documentation](https://doc.owncloud.com/ocis/next/)

## Original README Content

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cs3org/cs3apis).

### Installation

Head to [Documentation](https://reva.link) for documentation or [download](https://github.com/cs3org/reva/releases) to get the latest available release.

### Build and run it yourself

You need to have [Go](https://golang.org/doc/install) (version 1.16 or higher), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed.

#### Run tests

Reva's codebase continuously undergoes testing at various levels.

**Unit tests:** Runs the `<unit>_test.go` files that appear next to `<unit>.go` files. Run all with `make test`.

**Integration tests (GRPC):** Requires Redis. See `tests/integration`.

```sh
export REDIS_ADDRESS=127.0.0.1:6379
make test-integration
```

**Litmus tests (WebDAV):** Uses ownCloud's litmus Docker image for WebDAV testing.

**Acceptance tests:** See [Running Acceptance Tests](https://github.com/cs3org/reva/tree/edge/tests/acceptance).

### Daily releases

On every commit on the master branch a new release will be created and available at [daily releases](https://reva-releases.web.cern.ch/reva-releases).

### Major versions

- **1.x versions:** The `master` branch is the stable development branch.
- **2.x versions:** The `edge` branch is for the next version based on the "Spaces" concept.

### Plugin development

Checkout the [Tutorials](https://reva.link/docs/tutorials/).

## Community & Support

**[Star](https://github.com/owncloud/reva)** this repo and **Watch** for release notifications!

- [ownCloud Website](https://owncloud.com)
- [Community Discussions](https://github.com/orgs/owncloud/discussions)
- [Matrix Chat](https://app.element.io/#/room/#owncloud:matrix.org)
- [Documentation](https://doc.owncloud.com)
- [Enterprise Support](https://owncloud.com/contact-us/)
- [OSPO Home](https://kiteworks.com/opensource)

## Contributing

We welcome contributions! Please read the [Contributing Guidelines](CONTRIBUTING.md)
and our [Code of Conduct](CODE_OF_CONDUCT.md) before getting started.

### Workflow

- **Rebase Early, Rebase Often!** We use a rebase workflow. Always rebase on the target branch before submitting a PR.
- **Dependabot**: Automated dependency updates are managed via Dependabot. Review and merge dependency PRs promptly.
- **Signed Commits**: All commits **must** be PGP/GPG signed. See [GitHub's signing guide](https://docs.github.com/en/authentication/managing-commit-signature-verification).
- **DCO Sign-off**: Every commit must carry a `Signed-off-by` line:
  ```
  git commit -s -S -m "your commit message"
  ```
- **GitHub Actions Policy**: Workflows may only use actions that are (a) owned by `owncloud`, (b) created by GitHub (`actions/*`), or (c) verified in the GitHub Marketplace.

## Security

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities at **<https://security.owncloud.com>** -- see [SECURITY.md](SECURITY.md).

Bug bounty: [YesWeHack ownCloud Program](https://yeswehack.com/programs/owncloud-bug-bounty-program)

## License

This project is licensed under the [Apache-2.0](LICENSE).

## About the ownCloud OSPO

The [Kiteworks Open Source Program Office](https://kiteworks.com/opensource), operating under
the [ownCloud](https://owncloud.com) brand, launched on May 5, 2026, to steward the open source
ecosystem around ownCloud's products. The OSPO ensures transparent governance, license compliance,
community health, and sustainable collaboration between the open source community and
[Kiteworks](https://www.kiteworks.com), which acquired ownCloud in 2023.

- **OSPO Home**: <https://kiteworks.com/opensource>
- **GitHub**: <https://github.com/owncloud>
- **ownCloud**: <https://owncloud.com>

For questions about the OSPO or licensing, contact ospo@kiteworks.com.

> **License status:** This repository is already licensed under Apache-2.0 -- the OSPO target license.
> No migration is required.
