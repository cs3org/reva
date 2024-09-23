
---
title: "v2.19.9"
linkTitle: "v2.19.9"
weight: 40
description: >
  Changelog for Reva v2.19.9 (2024-09-23)
---

Changelog for reva 2.19.9 (2024-09-23)
=======================================

The following sections list the changes in reva 2.19.9 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #4842: Fix micro ocdav service init and registration
*   Fix #4862: Fix nats encoding

Details
-------

*   Bugfix #4842: Fix micro ocdav service init and registration

   We no longer call Init to configure default options because it was replacing the existing
   options.

   https://github.com/cs3org/reva/pull/4842
   https://github.com/cs3org/reva/pull/4774

*   Bugfix #4862: Fix nats encoding

   Encode nats-js-kv keys. This got lost by a dependency bump.

   https://github.com/cs3org/reva/pull/4862
   https://github.com/cs3org/reva/pull/4678

