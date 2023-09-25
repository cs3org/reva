
---
title: "v2.16.1"
linkTitle: "v2.16.1"
weight: 40
description: >
  Changelog for Reva v2.16.1 (2023-09-25)
---

Changelog for reva 2.16.1 (2023-09-25)
=======================================

The following sections list the changes in reva 2.16.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4214: Make appctx package compatible with go v1.21
*   Fix #4214: Always pass adjusted default nats options

Details
-------

*   Bugfix #4214: Make appctx package compatible with go v1.21

   Backported fix from edge. See https://tip.golang.org/doc/go1.21#reflect

   https://github.com/cs3org/reva/pull/4214

*   Bugfix #4214: Always pass adjusted default nats options

   The nats-js store will now automatically reconnect.

   https://github.com/cs3org/reva/pull/4214

