
---
title: "v2.27.1"
linkTitle: "v2.27.1"
weight: 40
description: >
  Changelog for Reva v2.27.1 (2025-01-09)
---

Changelog for reva 2.27.1 (2025-01-09)
=======================================

The following sections list the changes in reva 2.27.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #5033: Fix ocm wildcards
*   Fix #5031: Allow to accepted invite after it was once deleted
*   Fix #5026: Delete Blobs when Space is deleted
*   Enh #5025: Allow wildcards in OCM domains
*   Enh #5023: Notification feature toggle
*   Enh #4990: Allow locking via ocm
*   Enh #5032: Add SendEmailsEvent

Details
-------

*   Bugfix #5033: Fix ocm wildcards

   Ocm wildcards were not working properly. We now overwrite the wildcard values with the actual
   domain.

   https://github.com/cs3org/reva/pull/5033

*   Bugfix #5031: Allow to accepted invite after it was once deleted

   Allowed to accepted invite even after it was once deleted on the invite receiver or invite
   creation side.

   https://github.com/owncloud/ocis/issues/10813
   https://github.com/cs3org/reva/pull/5031

*   Bugfix #5026: Delete Blobs when Space is deleted

   Delete all blobs of a space when the space is deleted.

   https://github.com/cs3org/reva/pull/5026

*   Enhancement #5025: Allow wildcards in OCM domains

   When verifiying domains, allow wildcards in the domain name. This will not work when using
   `verify-request-hostname`

   https://github.com/cs3org/reva/pull/5025

*   Enhancement #5023: Notification feature toggle

   Adds a feature toggle for the notification settings.

   https://github.com/cs3org/reva/pull/5023

*   Enhancement #4990: Allow locking via ocm

   Implement locking endpoints so files can be locked and unlocked via ocm.

   https://github.com/cs3org/reva/pull/4990

*   Enhancement #5032: Add SendEmailsEvent

   Adds SendEmailsEvent that is used to trigger the sending of group emails.

   https://github.com/cs3org/reva/pull/5032

