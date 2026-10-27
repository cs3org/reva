# reva.link

The website of the reva project, built with [Hugo](https://gohugo.io). It needs Hugo 0.160 or newer and nothing else: no Node, no theme submodule.

## Build locally

```
make gen-doc        # from the repository root: regenerate the component pages and data
make docs-serve     # serve the site on http://localhost:1313
```

## What is generated

`make gen-doc` runs `tools/generate-documentation`. Do not edit its output by hand.

| Output | Source |
| --- | --- |
| `content/en/docs/components/` | Every package under `pkg/` and `internal/` that registers a plugin, has a `README.md`, or has documented settings. The page shows the package doc comment, the README, and a table of the `mapstructure` fields of its config struct, with field comments as descriptions and defaults read from `ApplyDefaults`. |
| `data/generated/releases.json` | The directories in `changelog/`. |
| `data/generated/activity.json` | `git log`. |
| `data/generated/stats.json` | `VERSION`, `RELEASE_DATE`, and the component counts. |
| `data/generated/contributors.json` | The GitHub API. Refreshed only by `make gen-doc-contributors` (set `GITHUB_TOKEN` to avoid the rate limit). |

To improve a component page, add a doc comment or a `README.md` to the package, or comment the fields of its config struct.

A package `README.md` can start with a header that names and groups the component:

```
---
subsystem_name: Storage drivers
subsystem_category: Storage
---
```

`subsystem_name` replaces the package name as the page title. `subsystem_category` groups components inside their kind in the sidebar and the lists (free text, case is ignored). In the README of a driver family's registry package (for example `pkg/storage/fs/registry`), the header names the family and gives its category to all its drivers.

## What is written by hand

- `content/en/`: all other pages. The release notes in `content/en/docs/changelog/` come from `tools/prepare-release`.
- `data/architecture.yaml`: the architecture diagram and its request traces. The build fails if a node links to a page that does not exist.
- `data/history.yaml`: the milestones on the history page.
- `data/deployments.yaml`: the deployments page.
- `static/quickstart/`: the configuration that the quick start uses. Run the quick start again after you change it.

## HTTP API page

Hugo fetches the OpenAPI document of `params.openapi` in `hugo.toml` at build time and publishes it with the site. If the fetch fails, the build continues and the page explains how to get the document from a deployment. To build against another deployment:

```
HUGO_PARAMS_OPENAPI=http://localhost:19001/openapi.json hugo server
```

## Deployment

Netlify builds the site on every change to `docs/` (see `netlify.toml`).
