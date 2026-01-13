# Reva CLI Integration Tests

This directory contains integration tests using the Reva CLI. These tests are designed to run in Reva's CI/CD pipeline, but also to be integrated in the pipeline of software Reva depends on, such as EOS. This allows upstream developers to catch changes that break Reva workflows immediately.

## Overview

These tests use the [Judo](https://github.com/intuit/judo) framework to execute CLI commands and verify their output. The tests are organized as YAML files, each focusing on a specific workflow.

## Prerequisites

- Judo installed: `npm install -g @intuit/judo`
- Reva and revad have been built (`make reva`, `make revad`)

## Running Tests

Tests can be run using `make test-reva-cli`.

## Overview of test coverage

This test suite aims covers the following methods:
* CreateDir, TouchFile, Delete, Move, GetMD, ListFolder, Upload, Download
* ListRevisions, DownloadRevision, RestoreRevision
* ListRecycle, RestoreRecycleItem, PurgeRecycleItem, EmptyRecycle
* AddGrant, RemoveGrant, UpdateGrant, ListGrants

In the future, it would be nice to also test the following:
* GetHome, CreateHome
* GetQuota
* SetArbitraryMetadata, UnsetArbitraryMetada
* SetLock, GetLock, RefreshLock, Unlock
* DenyGrant

### CI/CD Pipelines

The Reva CI/CD pipeline uses localfs as a backend, while EOS will obviously use a different config, with EOS as its storage backend. The Reva pipeline tests are defined in [test-reva-cli.yml](/.github/workflows/test-reva-cli.yml)

## Test Structure

Each test file uses the Judo format:

```yaml
run:
  uploadFile:
    prerequisiteCwd: .
    prerequisites:
      - 'echo "Hello World" > local-test-file.txt'
    command:  'cmd/reva/reva -insecure -host={{REVANODE}} -token-file=token upload local-test-file.txt  /localfs/my-test-file.txt'
    expectCode: 0
```
