
---
title: "v2.27.4"
linkTitle: "v2.27.4"
weight: 40
description: >
  Changelog for Reva v2.27.4 (2025-02-03)
---

Changelog for reva 2.27.4 (2025-02-03)
=======================================

The following sections list the changes in reva 2.27.4 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #5061: OCM Wildcards
*   Fix #5055: Fix view&download permission issue

Details
-------

*   Bugfix #5061: OCM Wildcards

   Fix using ocm wildcards. Do not overwrite cached provider with actual value

   https://github.com/cs3org/reva/pull/5061

*   Bugfix #5055: Fix view&download permission issue

   When opening files with view&download permission (aka read), the appprovider would falsely
   issue a secureview token. This is fixed now.

   https://github.com/cs3org/reva/pull/5055

